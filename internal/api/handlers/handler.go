package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bhavyavj/oi-assistant/internal/core"
	"github.com/bhavyavj/oi-assistant/internal/excel"
	"github.com/bhavyavj/oi-assistant/internal/models"
	"github.com/bhavyavj/oi-assistant/internal/storage"
)

type Handler struct {
	store    *storage.RedisStore
	analyzer *core.Analyzer
	log      *slog.Logger
}

func New(store *storage.RedisStore, analyzer *core.Analyzer, log *slog.Logger) *Handler {
	return &Handler{store: store, analyzer: analyzer, log: log}
}

// UploadExcel handles multipart file upload, parses OI data, caches it in Redis.
func (h *Handler) UploadExcel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonError(w, "file too large (max 10MB)", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("options_file")
	if err != nil {
		jsonError(w, "field 'options_file' required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	symbol := strings.ToUpper(strings.TrimSpace(r.FormValue("symbol")))
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		jsonError(w, "only .xlsx, .xls or .csv files accepted", http.StatusBadRequest)
		return
	}

	// For wide-format CSVs (common from NSE downloads), prefer symbol inferred from the filename
	// over the form value (which may be left as the default "NIFTY").
	// This makes the system "capable enough to fetch it from the file" as requested.
	if ext == ".csv" {
		if inferred := inferSymbolFromFilename(header.Filename); inferred != "" {
			if symbol == "" || symbol == "NIFTY" {
				symbol = inferred
			}
		}
	}

	tmpName := "oi-*.xlsx"
	if ext == ".csv" {
		tmpName = "oi-*.csv"
	}
	tmp, err := os.CreateTemp("", tmpName)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := tmp.ReadFrom(file); err != nil {
		jsonError(w, "failed to read file", http.StatusInternalServerError)
		return
	}
	tmp.Close()

	var records []models.OptionRecord
	if ext == ".csv" {
		records, err = parseCSV(tmp.Name(), symbol)
	} else {
		records, err = excel.Parse(tmp.Name())
	}
	if err != nil {
		jsonError(w, "parse error: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	// If symbol still not set after parsing, require it explicitly
	if symbol == "" {
		// check if all records got a real symbol from the file
		for _, r := range records {
			if r.Symbol == "" || r.Symbol == "UNKNOWN" {
				jsonError(w, "symbol is required: pass -F 'symbol=NIFTY' or enter it in the UI", http.StatusBadRequest)
				return
			}
		}
	} else {
		// override symbol on all records (wide CSV has no symbol column)
		for i := range records {
			records[i].Symbol = symbol
		}
	}

	bySymbol := make(map[string][]models.OptionRecord)
	for _, rec := range records {
		bySymbol[rec.Symbol] = append(bySymbol[rec.Symbol], rec)
	}
	for sym, recs := range bySymbol {
		if err := h.store.SaveRecords(r.Context(), sym, recs); err != nil {
			h.log.Error("redis save failed", "symbol", sym, "error", err)
		}
		_ = h.store.DeleteSignals(r.Context(), sym)
	}

	h.log.Info("upload processed", "records", len(records), "symbols", len(bySymbol))
	jsonOK(w, map[string]any{
		"records": len(records),
		"symbols": keys(bySymbol),
		"message": "upload successful",
	})
}

// Analyse retrieves cached OI data and returns metrics + signals.
func (h *Handler) Analyse(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(r.URL.Query().Get("symbol"))
	if symbol == "" {
		jsonError(w, "query param 'symbol' required", http.StatusBadRequest)
		return
	}

	cached, err := h.store.GetAnalysis(r.Context(), symbol)
	if err == nil && cached != nil {
		jsonOK(w, cached)
		return
	}

	records, err := h.store.GetRecords(r.Context(), symbol)
	if err != nil {
		jsonError(w, "redis error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(records) == 0 {
		jsonError(w, "no data for symbol "+symbol+"; upload a CSV first", http.StatusNotFound)
		return
	}

	signals := h.analyzer.Analyze(r.Context(), records)
	overall, expiries := core.CalcMetrics(records)
	resp := &models.AnalyseResponse{
		Symbol:   symbol,
		Cached:   false,
		Metrics:  overall,
		Expiries: expiries,
		Signals:  signals,
	}

	if err := h.store.SaveAnalysis(r.Context(), symbol, resp); err != nil {
		h.log.Warn("failed to cache analysis", "error", err)
	}

	jsonOK(w, resp)
}

// Health is a simple liveness probe.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// parseCSV supports two formats:
//  1. Wide NSE format: CALLS cols | STRIKE | PUTS cols
//  2. Tall normalized format: Symbol/Strike/OptionType/... columns
func parseCSV(path string, symbolOverride string) ([]models.OptionRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("csv has no data")
	}

	// Find header row: first row containing "strike" or "strike price"
	headerIdx := 0
	for i, row := range rows {
		for _, cell := range row {
			n := strings.ToLower(strings.TrimSpace(cell))
			if n == "strike" || n == "strike price" {
				headerIdx = i
				break
			}
		}
		if headerIdx == i && i > 0 {
			break
		}
	}
	header := rows[headerIdx]
	dataRows := rows[headerIdx+1:]

	strikeCol := -1
	for i, h := range header {
		n := strings.ToLower(strings.TrimSpace(h))
		if n == "strike" || n == "strike price" || n == "strikeprice" {
			strikeCol = i
			break
		}
	}
	if strikeCol < 0 {
		return nil, fmt.Errorf("no strike column found")
	}

	// Detect wide format: CALLS/PUTS label row above, or no option-type column
	isWide := false
	if headerIdx > 0 {
		for _, cell := range rows[headerIdx-1] {
			n := strings.ToLower(strings.TrimSpace(cell))
			if n == "calls" || n == "puts" {
				isWide = true
				break
			}
		}
	}
	if !isWide {
		hasOptType := false
		for _, h := range header {
			n := strings.ToLower(strings.TrimSpace(h))
			if n == "option type" || n == "type" || n == "ce/pe" || n == "optiontype" {
				hasOptType = true
				break
			}
		}
		if !hasOptType {
			isWide = true
		}
	}

	if isWide {
		return parseWideCSV(header, dataRows, strikeCol, symbolOverride)
	}
	return parseTallCSV(header, dataRows, strikeCol, symbolOverride)
}

func parseWideCSV(header []string, dataRows [][]string, strikeCol int, sym string) ([]models.OptionRecord, error) {
	if sym == "" {
		sym = "UNKNOWN"
	}

	// NSE wide layout (left of STRIKE = CE, right of STRIKE = PE):
	//   CE: OI, CHNG IN OI, VOLUME, IV, LTP, ...
	//   PE: ..., LTP, IV, VOLUME, CHNG IN OI, OI
	byName := make(map[string][]int)
	for i, h := range header {
		n := strings.ToLower(strings.TrimSpace(h))
		byName[n] = append(byName[n], i)
	}

	pick := func(name string, leftOfStrike bool) int {
		idxs := byName[name]
		if len(idxs) == 0 {
			return -1
		}
		if leftOfStrike {
			for j := len(idxs) - 1; j >= 0; j-- {
				if idxs[j] < strikeCol {
					return idxs[j]
				}
			}
		} else {
			for _, idx := range idxs {
				if idx > strikeCol {
					return idx
				}
			}
		}
		return -1
	}

	ceOI := pick("oi", true)
	ceChng := pick("chng in oi", true)
	ceVol := pick("volume", true)
	ceLtp := pick("ltp", true)
	ceIV := pick("iv", true)

	peOI := pick("oi", false)
	peChng := pick("chng in oi", false)
	peVol := pick("volume", false)
	peLtp := pick("ltp", false)
	peIV := pick("iv", false)

	if ceOI < 0 || peOI < 0 {
		return nil, fmt.Errorf("wide CSV: could not locate OI columns (ceOI=%d, peOI=%d)", ceOI, peOI)
	}

	var recs []models.OptionRecord
	for _, row := range dataRows {
		strike := cleanNum(getCol(row, strikeCol))
		if strike == 0 {
			continue
		}

		if oi := cleanInt(getCol(row, ceOI)); oi > 0 {
			chng := int(cleanNum(getCol(row, ceChng)))
			prev := maxInt(0, oi-chng)
			rec := models.OptionRecord{
				Symbol: sym, StrikePrice: strike, OptionType: "CE",
				OI: int64(oi), PrevOI: int64(prev),
				Volume: int64(cleanInt(getCol(row, ceVol))),
				LTP:    cleanNum(getCol(row, ceLtp)),
				IV:     cleanNum(getCol(row, ceIV)),
			}
			if rec.PrevOI > 0 {
				rec.OIChange = float64(rec.OI-rec.PrevOI) / float64(rec.PrevOI) * 100
			}
			recs = append(recs, rec)
		}

		if oi := cleanInt(getCol(row, peOI)); oi > 0 {
			chng := int(cleanNum(getCol(row, peChng)))
			prev := maxInt(0, oi-chng)
			rec := models.OptionRecord{
				Symbol: sym, StrikePrice: strike, OptionType: "PE",
				OI: int64(oi), PrevOI: int64(prev),
				Volume: int64(cleanInt(getCol(row, peVol))),
				LTP:    cleanNum(getCol(row, peLtp)),
				IV:     cleanNum(getCol(row, peIV)),
			}
			if rec.PrevOI > 0 {
				rec.OIChange = float64(rec.OI-rec.PrevOI) / float64(rec.PrevOI) * 100
			}
			recs = append(recs, rec)
		}
	}
	return recs, nil
}

func parseTallCSV(header []string, dataRows [][]string, strikeCol int, symbolOverride string) ([]models.OptionRecord, error) {
	sym, optType, oi, prevOI, vol, ltp, iv := -1, -1, -1, -1, -1, -1, -1
	for i, h := range header {
		n := strings.ToLower(strings.TrimSpace(h))
		switch n {
		case "symbol", "scrip":
			sym = i
		case "option type", "type", "ce/pe", "optiontype":
			optType = i
		case "oi", "open interest", "openinterest":
			oi = i
		case "prev oi", "previous oi", "prevoi":
			prevOI = i
		case "volume", "vol":
			vol = i
		case "ltp", "last price", "lastprice":
			ltp = i
		case "iv", "implied volatility", "impliedvolatility":
			iv = i
		}
	}

	var recs []models.OptionRecord
	for _, row := range dataRows {
		if len(row) == 0 {
			continue
		}
		s := getCol(row, sym)
		if s == "" {
			s = symbolOverride
		}
		if s == "" {
			s = "UNKNOWN"
		}
		strike := cleanNum(getCol(row, strikeCol))
		if strike == 0 {
			continue
		}
		ot := strings.ToUpper(getCol(row, optType))
		if ot != "CE" && ot != "PE" {
			continue
		}

		oiVal := cleanInt(getCol(row, oi))
		prev := maxInt(0, cleanInt(getCol(row, prevOI)))
		rec := models.OptionRecord{
			Symbol: s, StrikePrice: strike, OptionType: ot,
			OI: int64(oiVal), PrevOI: int64(prev),
			Volume: int64(cleanInt(getCol(row, vol))),
			LTP:    cleanNum(getCol(row, ltp)),
			IV:     cleanNum(getCol(row, iv)),
		}
		if rec.PrevOI > 0 {
			rec.OIChange = float64(rec.OI-rec.PrevOI) / float64(rec.PrevOI) * 100
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

func getCol(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func cleanNum(s string) float64 {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), `"`, "")
	if s == "" || s == "-" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func cleanInt(s string) int {
	return int(cleanNum(s))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// inferSymbolFromFilename extracts symbol from common NSE download filenames.
// e.g. "option-chain-ED-KPITTECH-30-Jun-2026.csv" -> "KPITTECH"
func inferSymbolFromFilename(filename string) string {
	if filename == "" {
		return ""
	}
	base := filepath.Base(filename)
	base = strings.ToUpper(base)
	re := regexp.MustCompile(`OPTION-CHAIN-ED-([A-Z0-9]+)-`)
	if m := re.FindStringSubmatch(base); len(m) > 1 {
		return m[1]
	}
	// other patterns
	re = regexp.MustCompile(`([A-Z0-9]{3,})_?OPTION`)
	if m := re.FindStringSubmatch(base); len(m) > 1 {
		return m[1]
	}
	return ""
}

