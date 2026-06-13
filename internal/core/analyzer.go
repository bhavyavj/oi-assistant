package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/bhavyavj/oi-assistant/internal/llm"
	"github.com/bhavyavj/oi-assistant/internal/models"
)

type Analyzer struct {
	llm       llm.Client
	threshold float64
	semaphore chan struct{}
	log       *slog.Logger
}

func NewAnalyzer(client llm.Client, threshold float64, maxConcurrency int, log *slog.Logger) *Analyzer {
	return &Analyzer{
		llm:       client,
		threshold: threshold,
		semaphore: make(chan struct{}, maxConcurrency),
		log:       log,
	}
}

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
			select {
			case a.semaphore <- struct{}{}:
				defer func() { <-a.semaphore }()
			case <-ctx.Done():
				signals[idx] = fallbackSignal(r)
				return
			}
			var sig models.TradeSignal
			if a.llm == nil {
				sig = fallbackSignal(r)
			} else {
				var err error
				sig, err = a.generateSignal(ctx, r)
				if err != nil {
					a.log.Warn("LLM failed, using fallback", "symbol", r.Symbol, "error", err)
					sig = fallbackSignal(r)
				}
			}
			signals[idx] = sig
		}(i, rec)
	}
	wg.Wait()
	return signals
}

func (a *Analyzer) generateSignal(ctx context.Context, r models.OptionRecord) (models.TradeSignal, error) {
	raw, err := a.llm.Complete(ctx, buildPrompt(r))
	if err != nil {
		return models.TradeSignal{}, err
	}
	sig, err := parseResponse(raw, r)
	if err != nil {
		return fallbackSignal(r), nil
	}
	sig.Source = "llm"
	return sig, nil
}

func buildPrompt(r models.OptionRecord) string {
	dir := "increased"
	if r.OIChange < 0 {
		dir = "decreased"
	}
	return fmt.Sprintf(`You are an options trading analyst. Analyze this OI data and respond in JSON only.

Symbol: %s | %s %s | Expiry: %s
OI: %d (prev %d) | OI Change: %.2f%% (%s)
Volume: %d | LTP: %.2f | IV: %.2f

{"action":"BUY|SELL|WATCH","rationale":"one sentence","confidence":"HIGH|MEDIUM|LOW"}`,
		r.Symbol, r.OptionType, fmt.Sprintf("%.0f", r.StrikePrice), r.Expiry,
		r.OI, r.PrevOI, r.OIChange, dir, r.Volume, r.LTP, r.IV)
}

var jsonRe = regexp.MustCompile(`(?s)\{.*\}`)

func parseResponse(raw string, r models.OptionRecord) (models.TradeSignal, error) {
	match := jsonRe.FindString(raw)
	if match == "" {
		return models.TradeSignal{}, fmt.Errorf("no JSON")
	}
	var out struct {
		Action     string `json:"action"`
		Rationale  string `json:"rationale"`
		Confidence string `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(match), &out); err != nil {
		return models.TradeSignal{}, err
	}
	out.Action = strings.ToUpper(out.Action)
	out.Confidence = strings.ToUpper(out.Confidence)
	if !map[string]bool{"BUY": true, "SELL": true, "WATCH": true}[out.Action] {
		return models.TradeSignal{}, fmt.Errorf("invalid action %q", out.Action)
	}
	if !map[string]bool{"HIGH": true, "MEDIUM": true, "LOW": true}[out.Confidence] {
		out.Confidence = "LOW"
	}
	if len(out.Rationale) < 10 {
		return models.TradeSignal{}, fmt.Errorf("rationale too short")
	}
	return models.TradeSignal{
		Symbol: r.Symbol, StrikePrice: r.StrikePrice, OptionType: r.OptionType, Expiry: r.Expiry,
		Action: out.Action, Rationale: out.Rationale, Confidence: out.Confidence,
	}, nil
}

// CalcMetrics returns overall metrics + per-expiry breakdown.
func CalcMetrics(records []models.OptionRecord) (overall models.OIMetrics, expiries []models.OIMetrics) {
	// Group by expiry
	byExpiry := make(map[string][]models.OptionRecord)
	for _, r := range records {
		byExpiry[r.Expiry] = append(byExpiry[r.Expiry], r)
	}

	expiryKeys := make([]string, 0, len(byExpiry))
	for k := range byExpiry {
		expiryKeys = append(expiryKeys, k)
	}
	sort.Strings(expiryKeys)

	for _, exp := range expiryKeys {
		m := calcMetricsForGroup(byExpiry[exp])
		m.Expiry = exp
		expiries = append(expiries, m)
	}

	overall = calcMetricsForGroup(records)
	return overall, expiries
}

func calcMetricsForGroup(records []models.OptionRecord) models.OIMetrics {
	ceOI := map[float64]int64{}
	peOI := map[float64]int64{}
	var ceIVSum, peIVSum float64
	var ceIVCount, peIVCount int

	for _, r := range records {
		if r.OptionType == "CE" {
			ceOI[r.StrikePrice] += r.OI
			if r.IV > 0 {
				ceIVSum += r.IV
				ceIVCount++
			}
		} else if r.OptionType == "PE" {
			peOI[r.StrikePrice] += r.OI
			if r.IV > 0 {
				peIVSum += r.IV
				peIVCount++
			}
		}
	}

	strikeSet := map[float64]struct{}{}
	for k := range ceOI {
		strikeSet[k] = struct{}{}
	}
	for k := range peOI {
		strikeSet[k] = struct{}{}
	}
	strikes := make([]float64, 0, len(strikeSet))
	for k := range strikeSet {
		strikes = append(strikes, k)
	}

	var totalCE, totalPE int64
	for _, v := range ceOI {
		totalCE += v
	}
	for _, v := range peOI {
		totalPE += v
	}

	var pcr float64
	if totalCE > 0 {
		pcr = float64(totalPE) / float64(totalCE)
	}

	// Max pain: strike where total option seller loss is minimized
	var maxPain float64
	minLoss := math.MaxFloat64
	for _, s := range strikes {
		var loss float64
		for _, k := range strikes {
			if s > k {
				loss += (s - k) * float64(ceOI[k])
			}
			if k > s {
				loss += (k - s) * float64(peOI[k])
			}
		}
		if loss < minLoss {
			minLoss = loss
			maxPain = s
		}
	}

	// IV skew: positive = CE IV > PE IV = calls pricier = bearish/event risk
	var ivSkew float64
	if ceIVCount > 0 && peIVCount > 0 {
		ivSkew = (ceIVSum / float64(ceIVCount)) - (peIVSum / float64(peIVCount))
	}

	bias, biasReason := determineBias(pcr, ivSkew)

	return models.OIMetrics{
		PCR:          math.Round(pcr*100) / 100,
		MaxPain:      maxPain,
		IVSkew:       math.Round(ivSkew*100) / 100,
		Bias:         bias,
		BiasReason:   biasReason,
		TopCEStrikes: topN(ceOI, 3),
		TopPEStrikes: topN(peOI, 3),
		TotalCEOI:    totalCE,
		TotalPEOI:    totalPE,
	}
}

// determineBias produces a bias label + single-sentence reason from PCR and IV skew.
func determineBias(pcr, ivSkew float64) (string, string) {
	// PCR > 1.2 → more put OI = put writing dominance = bullish
	// PCR < 0.8 → more call OI = call writing dominance = bearish
	// IV skew > 2 → calls pricier than puts = market expecting upside breakout or event vol
	// IV skew < -2 → puts pricier = downside hedging
	switch {
	case pcr > 1.3 && ivSkew < 2:
		return "Bullish", fmt.Sprintf("PCR %.2f indicates heavy put writing; institutions are selling puts (expecting support).", pcr)
	case pcr < 0.8 && ivSkew > -2:
		return "Bearish", fmt.Sprintf("PCR %.2f indicates call OI dominance; resistance building above.", pcr)
	case ivSkew > 3:
		return "Bullish", fmt.Sprintf("CE IV premium (skew +%.1f) signals upside demand; calls significantly pricier than puts.", ivSkew)
	case ivSkew < -3:
		return "Bearish", fmt.Sprintf("PE IV premium (skew %.1f) signals downside hedging; puts significantly pricier than calls.", ivSkew)
	case pcr >= 0.8 && pcr <= 1.3:
		return "Neutral", fmt.Sprintf("PCR %.2f is balanced; no strong directional conviction from OI.", pcr)
	default:
		return "Neutral", fmt.Sprintf("Mixed signals: PCR %.2f, IV skew %.2f.", pcr, ivSkew)
	}
}

func topN(m map[float64]int64, n int) []float64 {
	type kv struct {
		strike float64
		oi     int64
	}
	kvs := make([]kv, 0, len(m))
	for k, v := range m {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].oi > kvs[j].oi })
	if len(kvs) < n {
		n = len(kvs)
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = kvs[i].strike
	}
	return out
}

func fallbackSignal(r models.OptionRecord) models.TradeSignal {
	action := "WATCH"
	confidence := "LOW"
	dir := "increased"
	if r.OIChange < 0 {
		dir = "decreased"
	}
	rationale := fmt.Sprintf("OI %s by %.1f%%; monitoring recommended.", dir, math.Abs(r.OIChange))

	switch {
	case r.OIChange > 50 && r.OptionType == "PE":
		action, confidence = "BUY", "MEDIUM"
		rationale = fmt.Sprintf("Heavy put writing (+%.1f%%) — institutions expect support at %.0f (bullish).", r.OIChange, r.StrikePrice)
	case r.OIChange > 50 && r.OptionType == "CE":
		action, confidence = "WATCH", "MEDIUM"
		rationale = fmt.Sprintf("CE OI build-up (+%.1f%%) at %.0f — call writing resistance; watch for breakout.", r.OIChange, r.StrikePrice)
	case r.OIChange < -30 && r.OptionType == "PE":
		action, confidence = "SELL", "MEDIUM"
		rationale = fmt.Sprintf("PE unwinding (%.1f%%) at %.0f — put longs exiting; bearish breakdown risk.", r.OIChange, r.StrikePrice)
	case r.OIChange < -30 && r.OptionType == "CE":
		action, confidence = "WATCH", "MEDIUM"
		rationale = fmt.Sprintf("CE unwinding (%.1f%%) at %.0f — call shorts covering; potential bullish breakout.", r.OIChange, r.StrikePrice)
	}

	return models.TradeSignal{
		Symbol: r.Symbol, StrikePrice: r.StrikePrice, OptionType: r.OptionType, Expiry: r.Expiry,
		Action: action, Rationale: rationale, Confidence: confidence, Source: "fallback",
	}
}

// CompletePrompt delegates completions directly to the analyzer's LLM client.
func (a *Analyzer) CompletePrompt(ctx context.Context, prompt string) (string, error) {
	return a.llm.Complete(ctx, prompt)
}
