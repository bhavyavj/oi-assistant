package core

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/bhavyavj/oi-assistant/internal/models"
)

type mockLLMClient struct{}

func (m *mockLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	return `{"action":"BUY","rationale":"Strong put writing observed","confidence":"HIGH"}`, nil
}

func TestAnalyzer_AnalyzeRuleBased(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	// Create analyzer with threshold 20.0% and no active LLM client (or mock one that fails)
	analyzer := NewAnalyzer(nil, 20.0, 1, log)

	records := []models.OptionRecord{
		// 1. Put writing (bullish) -> OI change > 50% on PE
		{Symbol: "NIFTY", StrikePrice: 22000, OptionType: "PE", Expiry: "18-Jun-2026", OI: 1500, PrevOI: 900, OIChange: 66.67, Volume: 1000, LTP: 45.2, IV: 12.3},
		// 2. Below threshold -> OI change < 20%
		{Symbol: "NIFTY", StrikePrice: 22100, OptionType: "CE", Expiry: "18-Jun-2026", OI: 1100, PrevOI: 1000, OIChange: 10.0, Volume: 1000, LTP: 80.5, IV: 11.2},
		// 3. PE unwinding (bearish) -> OI change < -30% on PE
		{Symbol: "NIFTY", StrikePrice: 21800, OptionType: "PE", Expiry: "18-Jun-2026", OI: 500, PrevOI: 1000, OIChange: -50.0, Volume: 2000, LTP: 12.4, IV: 14.1},
	}

	signals := analyzer.Analyze(context.Background(), records)

	// Since records[1] is below the threshold of 20%, only records[0] and records[2] should generate signals.
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(signals))
	}

	// First signal should be NIFTY 22000 PE -> BUY (put writing)
	if signals[0].StrikePrice != 22000 || signals[0].OptionType != "PE" || signals[0].Action != "BUY" {
		t.Errorf("expected 22000 PE BUY signal, got %+v", signals[0])
	}

	// Second signal should be NIFTY 21800 PE -> SELL (unwinding)
	if signals[1].StrikePrice != 21800 || signals[1].OptionType != "PE" || signals[1].Action != "SELL" {
		t.Errorf("expected 21800 PE SELL signal, got %+v", signals[1])
	}
}

func TestAnalyzer_AnalyzeLLM(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockClient := &mockLLMClient{}
	
	// Create analyzer with threshold 20.0% and mock LLM client
	analyzer := NewAnalyzer(mockClient, 20.0, 1, log)

	records := []models.OptionRecord{
		{Symbol: "NIFTY", StrikePrice: 22000, OptionType: "CE", Expiry: "18-Jun-2026", OI: 1500, PrevOI: 900, OIChange: 66.67, Volume: 1000, LTP: 45.2, IV: 12.3},
	}

	signals := analyzer.Analyze(context.Background(), records)

	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}

	// Should prioritize LLM output
	if signals[0].Action != "BUY" || signals[0].Confidence != "HIGH" || signals[0].Source != "llm" {
		t.Errorf("expected LLM BUY signal with high confidence, got %+v", signals[0])
	}
}

func TestDetermineBias(t *testing.T) {
	// 1. Bullish bias from PCR
	bias, _ := determineBias(1.4, 0.0)
	if bias != "Bullish" {
		t.Errorf("expected Bullish bias for PCR 1.4, got %s", bias)
	}

	// 2. Bearish bias from PCR
	bias, _ = determineBias(0.5, 0.0)
	if bias != "Bearish" {
		t.Errorf("expected Bearish bias for PCR 0.5, got %s", bias)
	}

	// 3. Neutral bias
	bias, _ = determineBias(1.0, 0.0)
	if bias != "Neutral" {
		t.Errorf("expected Neutral bias for PCR 1.0, got %s", bias)
	}
}
