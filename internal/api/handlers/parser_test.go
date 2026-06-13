package handlers

import (
	"os"
	"testing"
)

func TestInferSymbolFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"option-chain-ED-RELIANCE-18-Jun-2026.csv", "RELIANCE"},
		{"option-chain-ED-NIFTY-30-Jun-2026.csv", "NIFTY"},
		{"KPITTECH_OPTION_CHAIN.csv", "KPITTECH"},
		{"random-file.csv", ""},
	}

	for _, tc := range tests {
		got := inferSymbolFromFilename(tc.filename)
		if got != tc.expected {
			t.Errorf("inferSymbolFromFilename(%q) = %q; expected %q", tc.filename, got, tc.expected)
		}
	}
}

func TestParseWideCSV(t *testing.T) {
	// Create a mock wide CSV file
	content := `CALLS,,,,,,STRIKE,PUTS,,,,,,
OI,CHNG IN OI,VOLUME,IV,LTP,,Strike Price,,LTP,IV,VOLUME,CHNG IN OI,OI
100,20,500,12.5,150.0,,22000.0,,80.0,14.2,400,-10,200
50,10,200,13.0,90.0,,22100.0,,120.0,13.8,600,40,400
`
	tmpFile, err := os.CreateTemp("", "mock-wide-*.csv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write mock content: %v", err)
	}
	tmpFile.Close()

	records, err := parseCSV(tmpFile.Name(), "NIFTY", "30-Jun-2026")
	if err != nil {
		t.Fatalf("parseCSV failed: %v", err)
	}

	// We expect 4 records: 2 strikes * (1 CE + 1 PE)
	if len(records) != 4 {
		t.Errorf("expected 4 records, got %d", len(records))
	}

	// Verify one call and one put
	var ceCount, peCount int
	for _, r := range records {
		if r.Symbol != "NIFTY" {
			t.Errorf("expected symbol NIFTY, got %s", r.Symbol)
		}
		if r.OptionType == "CE" {
			ceCount++
			if r.StrikePrice == 22000 {
				if r.OI != 100 || r.Volume != 500 || r.LTP != 150.0 {
					t.Errorf("CE 22000 field mismatch: %+v", r)
				}
			}
		} else if r.OptionType == "PE" {
			peCount++
			if r.StrikePrice == 22000 {
				if r.OI != 200 || r.Volume != 400 || r.LTP != 80.0 {
					t.Errorf("PE 22000 field mismatch: %+v", r)
				}
			}
		}
	}

	if ceCount != 2 || peCount != 2 {
		t.Errorf("expected 2 CEs and 2 PEs, got %d CEs and %d PEs", ceCount, peCount)
	}
}
