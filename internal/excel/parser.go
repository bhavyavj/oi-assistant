package excel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/bhavyavj/oi-assistant/internal/models"
)

// expectedHeaders maps canonical column name → accepted variants (lowercase).
var expectedHeaders = map[string][]string{
	"symbol":       {"symbol", "scrip"},
	"strike_price": {"strike price", "strike", "strikeprice"},
	"option_type":  {"option type", "type", "optiontype", "ce/pe"},
	"expiry":       {"expiry", "expiry date", "expiry_date"},
	"oi":           {"oi", "open interest", "openinterest"},
	"prev_oi":      {"prev oi", "previous oi", "prevoi"},
	"volume":       {"volume", "vol"},
	"ltp":          {"ltp", "last price", "lastprice"},
}

// Parse reads an Excel file from path and returns option records.
func Parse(path string) ([]models.OptionRecord, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open excel: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found")
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("get rows: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("file has no data rows")
	}

	colIdx, err := mapHeaders(rows[0])
	if err != nil {
		return nil, err
	}

	var records []models.OptionRecord
	for i, row := range rows[1:] {
		rec, err := parseRow(row, colIdx, i+2)
		if err != nil {
			continue // skip malformed rows
		}
		if rec.PrevOI > 0 {
			rec.OIChange = float64(rec.OI-rec.PrevOI) / float64(rec.PrevOI) * 100
		}
		records = append(records, rec)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no valid records parsed")
	}
	return records, nil
}

func mapHeaders(header []string) (map[string]int, error) {
	idx := make(map[string]int)
	for col, cell := range header {
		norm := strings.ToLower(strings.TrimSpace(cell))
		for canonical, variants := range expectedHeaders {
			for _, v := range variants {
				if norm == v {
					idx[canonical] = col
				}
			}
		}
	}
	required := []string{"symbol", "strike_price", "option_type", "oi"}
	for _, r := range required {
		if _, ok := idx[r]; !ok {
			return nil, fmt.Errorf("required column %q not found in header", r)
		}
	}
	return idx, nil
}

func parseRow(row []string, idx map[string]int, lineNum int) (models.OptionRecord, error) {
	get := func(key string) string {
		i, ok := idx[key]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	toFloat := func(s string) float64 {
		v, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
		return v
	}
	toInt := func(s string) int64 {
		v, _ := strconv.ParseInt(strings.ReplaceAll(s, ",", ""), 10, 64)
		return v
	}

	sym := get("symbol")
	if sym == "" {
		return models.OptionRecord{}, fmt.Errorf("line %d: empty symbol", lineNum)
	}

	ot := strings.ToUpper(get("option_type"))
	if ot != "CE" && ot != "PE" {
		return models.OptionRecord{}, fmt.Errorf("line %d: invalid option_type %q", lineNum, ot)
	}

	return models.OptionRecord{
		Symbol:      sym,
		StrikePrice: toFloat(get("strike_price")),
		OptionType:  ot,
		Expiry:      get("expiry"),
		OI:          toInt(get("oi")),
		PrevOI:      toInt(get("prev_oi")),
		Volume:      toInt(get("volume")),
		LTP:         toFloat(get("ltp")),
	}, nil
}
