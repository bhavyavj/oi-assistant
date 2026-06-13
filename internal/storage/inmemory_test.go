package storage

import (
	"context"
	"testing"
	"time"

	"github.com/bhavyavj/oi-assistant/internal/models"
)

func TestInMemoryStore_SaveAndGet(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore(1 * time.Hour)
	defer store.Close()

	symbol := "NIFTY"

	// 1. Test Spot Price
	expectedSpot := 22050.45
	if err := store.SaveSpotPrice(ctx, symbol, expectedSpot); err != nil {
		t.Fatalf("failed to save spot price: %v", err)
	}

	spot, err := store.GetSpotPrice(ctx, symbol)
	if err != nil {
		t.Fatalf("failed to get spot price: %v", err)
	}
	if spot != expectedSpot {
		t.Errorf("expected spot %f, got %f", expectedSpot, spot)
	}

	// 2. Test Records
	records := []models.OptionRecord{
		{Symbol: symbol, StrikePrice: 22000, OptionType: "CE", OI: 1000},
		{Symbol: symbol, StrikePrice: 22000, OptionType: "PE", OI: 1200},
	}
	if err := store.SaveRecords(ctx, symbol, records); err != nil {
		t.Fatalf("failed to save records: %v", err)
	}

	retrievedRecords, err := store.GetRecords(ctx, symbol)
	if err != nil {
		t.Fatalf("failed to get records: %v", err)
	}
	if len(retrievedRecords) != 2 {
		t.Errorf("expected 2 records, got %d", len(retrievedRecords))
	}
	if retrievedRecords[0].OI != 1000 || retrievedRecords[1].OI != 1200 {
		t.Errorf("records content mismatch")
	}

	// 3. Test Analysis Response
	analysis := &models.AnalyseResponse{
		Symbol:    symbol,
		SpotPrice: expectedSpot,
		Signals: []models.TradeSignal{
			{StrikePrice: 22000, OptionType: "PE", Action: "BUY", Confidence: "HIGH"},
		},
	}
	if err := store.SaveAnalysis(ctx, symbol, analysis); err != nil {
		t.Fatalf("failed to save analysis: %v", err)
	}

	retrievedAnalysis, err := store.GetAnalysis(ctx, symbol)
	if err != nil {
		t.Fatalf("failed to get analysis: %v", err)
	}
	if retrievedAnalysis == nil {
		t.Fatal("expected analysis response, got nil")
	}
	if !retrievedAnalysis.Cached {
		t.Error("expected retrieved analysis to be marked as cached")
	}
	if len(retrievedAnalysis.Signals) != 1 || retrievedAnalysis.Signals[0].Action != "BUY" {
		t.Errorf("analysis content mismatch")
	}

	// 4. Test Delete Signals
	if err := store.DeleteSignals(ctx, symbol); err != nil {
		t.Fatalf("failed to delete signals: %v", err)
	}
	deletedAnalysis, err := store.GetAnalysis(ctx, symbol)
	if err != nil {
		t.Fatalf("failed to get analysis after delete: %v", err)
	}
	if deletedAnalysis != nil {
		t.Error("expected analysis to be deleted (nil), got non-nil")
	}
}

func TestInMemoryStore_Expiration(t *testing.T) {
	ctx := context.Background()
	// Short TTL of 5 milliseconds
	store := NewInMemoryStore(5 * time.Millisecond)
	defer store.Close()

	symbol := "NIFTY"
	if err := store.SaveSpotPrice(ctx, symbol, 22000.0); err != nil {
		t.Fatalf("failed to save spot price: %v", err)
	}

	// Fetch immediately
	spot, err := store.GetSpotPrice(ctx, symbol)
	if err != nil || spot == 0 {
		t.Fatalf("failed to get spot price immediately: %v", err)
	}

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	expiredSpot, err := store.GetSpotPrice(ctx, symbol)
	if err != nil {
		t.Fatalf("failed to get spot price after expiration: %v", err)
	}
	if expiredSpot != 0 {
		t.Errorf("expected expired spot price to be 0, got %f", expiredSpot)
	}
}
