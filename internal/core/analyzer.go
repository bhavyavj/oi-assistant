package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"sync"

	"github.com/bhavyavj/oi-assistant/internal/llm"
	"github.com/bhavyavj/oi-assistant/internal/models"
)

// Analyzer filters significant OI changes and produces trade signals via LLM + fallback.
type Analyzer struct {
	llm            llm.Client
	threshold      float64 // OI change % to consider significant
	semaphore      chan struct{}
	log            *slog.Logger
}

func NewAnalyzer(client llm.Client, threshold float64, maxConcurrency int, log *slog.Logger) *Analyzer {
	return &Analyzer{
		llm:       client,
		threshold: threshold,
		semaphore: make(chan struct{}, maxConcurrency),
		log:       log,
	}
}

// Analyze finds records with significant OI change and generates signals concurrently.
func (a *Analyzer) Analyze(ctx context.Context, records []models.OptionRecord) []models.TradeSignal {
	var significant []models.OptionRecord
	for _, r := range records {
		if math.Abs(r.OIChange) >= a.threshold {
			significant = append(significant, r)
		}
	}

	if len(significant) == 0 {
		return nil
	}

	signals := make([]models.TradeSignal, len(significant))
	var wg sync.WaitGroup

	for i, rec := range significant {
		wg.Add(1)
		go func(idx int, r models.OptionRecord) {
			defer wg.Done()

			// Acquire semaphore slot
			select {
			case a.semaphore <- struct{}{}:
				defer func() { <-a.semaphore }()
			case <-ctx.Done():
				signals[idx] = fallbackSignal(r)
				return
			}

			sig, err := a.generateSignal(ctx, r)
			if err != nil {
				a.log.Warn("LLM failed, using fallback", "symbol", r.Symbol, "error", err)
				sig = fallbackSignal(r)
			}
			signals[idx] = sig
		}(i, rec)
	}

	wg.Wait()
	return signals
}

func (a *Analyzer) generateSignal(ctx context.Context, r models.OptionRecord) (models.TradeSignal, error) {
	prompt := buildPrompt(r)
	raw, err := a.llm.Complete(ctx, prompt)
	if err != nil {
		return models.TradeSignal{}, err
	}

	sig, err := parseResponse(raw, r)
	if err != nil {
		a.log.Warn("LLM output invalid, using fallback", "raw", raw, "error", err)
		return fallbackSignal(r), nil
	}
	sig.Source = "llm"
	return sig, nil
}

// buildPrompt constructs a structured prompt for the LLM.
func buildPrompt(r models.OptionRecord) string {
	direction := "increased"
	if r.OIChange < 0 {
		direction = "decreased"
	}
	return fmt.Sprintf(`You are an options trading analyst. Analyze this OI data and provide a trade suggestion.

Symbol: %s
Option Type: %s
Strike Price: %.2f
Expiry: %s
Current OI: %d
Previous OI: %d
OI Change: %.2f%% (%s)
Volume: %d
LTP: %.2f

Respond in JSON format only:
{
  "action": "BUY|SELL|WATCH",
  "rationale": "one sentence explanation",
  "confidence": "HIGH|MEDIUM|LOW"
}`,
		r.Symbol, r.OptionType, r.StrikePrice, r.Expiry,
		r.OI, r.PrevOI, r.OIChange, direction, r.Volume, r.LTP)
}

var jsonRe = regexp.MustCompile(`(?s)\{.*\}`)

// parseResponse extracts and validates the LLM JSON output.
func parseResponse(raw string, r models.OptionRecord) (models.TradeSignal, error) {
	match := jsonRe.FindString(raw)
	if match == "" {
		return models.TradeSignal{}, fmt.Errorf("no JSON in response")
	}

	var out struct {
		Action     string `json:"action"`
		Rationale  string `json:"rationale"`
		Confidence string `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(match), &out); err != nil {
		return models.TradeSignal{}, fmt.Errorf("unmarshal: %w", err)
	}

	out.Action = strings.ToUpper(out.Action)
	out.Confidence = strings.ToUpper(out.Confidence)

	validActions := map[string]bool{"BUY": true, "SELL": true, "WATCH": true}
	validConf := map[string]bool{"HIGH": true, "MEDIUM": true, "LOW": true}

	if !validActions[out.Action] {
		return models.TradeSignal{}, fmt.Errorf("invalid action: %q", out.Action)
	}
	if !validConf[out.Confidence] {
		out.Confidence = "LOW"
	}
	if len(out.Rationale) < 10 || len(out.Rationale) > 500 {
		return models.TradeSignal{}, fmt.Errorf("rationale too short or too long")
	}

	return models.TradeSignal{
		Symbol:      r.Symbol,
		StrikePrice: r.StrikePrice,
		OptionType:  r.OptionType,
		Action:      out.Action,
		Rationale:   out.Rationale,
		Confidence:  out.Confidence,
	}, nil
}

// fallbackSignal returns a deterministic signal when LLM is unavailable.
func fallbackSignal(r models.OptionRecord) models.TradeSignal {
	action := "WATCH"
	rationale := fmt.Sprintf("OI %s by %.1f%%; monitoring recommended.",
		func() string {
			if r.OIChange > 0 {
				return "increased"
			}
			return "decreased"
		}(), math.Abs(r.OIChange))
	confidence := "LOW"

	// Simple rule: large OI increase in CE → potential bullish signal, PE → bearish
	if r.OIChange > 50 {
		confidence = "MEDIUM"
		if r.OptionType == "CE" {
			action = "BUY"
			rationale = fmt.Sprintf("Strong OI build-up (+%.1f%%) in %s CE suggests bullish sentiment.", r.OIChange, r.Symbol)
		} else {
			action = "SELL"
			rationale = fmt.Sprintf("Strong OI build-up (+%.1f%%) in %s PE suggests bearish pressure.", r.OIChange, r.Symbol)
		}
	} else if r.OIChange < -30 {
		action = "WATCH"
		rationale = fmt.Sprintf("OI unwinding (%.1f%%) in %s %s; trend reversal possible.", r.OIChange, r.Symbol, r.OptionType)
	}

	return models.TradeSignal{
		Symbol:      r.Symbol,
		StrikePrice: r.StrikePrice,
		OptionType:  r.OptionType,
		Action:      action,
		Rationale:   rationale,
		Confidence:  confidence,
		Source:      "fallback",
	}
}
