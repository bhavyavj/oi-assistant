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
	"sort"
	"strconv"
	"strings"

	"github.com/bhavyavj/oi-assistant/internal/core"
	"github.com/bhavyavj/oi-assistant/internal/excel"
	"github.com/bhavyavj/oi-assistant/internal/models"
	"github.com/bhavyavj/oi-assistant/internal/nse"
	"github.com/bhavyavj/oi-assistant/internal/storage"
)

type Handler struct {
	store    storage.Store
	analyzer *core.Analyzer
	log      *slog.Logger
}

func New(store storage.Store, analyzer *core.Analyzer, log *slog.Logger) *Handler {
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

	// Always infer symbol from filename for .csv (NSE downloads are wide-format and the symbol
	// is in the filename, not the data). This takes precedence over the form input so the
	// user doesn't have to manually correct the symbol field.
	if ext == ".csv" {
		if inferred := inferSymbolFromFilename(header.Filename); inferred != "" {
			symbol = inferred
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
		defaultExpiry := inferExpiryFromFilename(header.Filename)
		records, err = parseCSV(tmp.Name(), symbol, defaultExpiry)
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
		if cached.SpotPrice == 0 {
			cached.SpotPrice, _ = h.store.GetSpotPrice(r.Context(), symbol)
		}
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

	spotPrice, _ := h.store.GetSpotPrice(r.Context(), symbol)

	signals := h.analyzer.Analyze(r.Context(), records)
	overall, expiries := core.CalcMetrics(records)
	resp := &models.AnalyseResponse{
		Symbol:    symbol,
		Cached:    false,
		SpotPrice: spotPrice,
		Metrics:   overall,
		Expiries:  expiries,
		Signals:   signals,
		Records:   records,
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
func parseCSV(path string, symbolOverride string, defaultExpiry string) ([]models.OptionRecord, error) {
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
		return parseWideCSV(header, dataRows, strikeCol, symbolOverride, defaultExpiry)
	}
	return parseTallCSV(header, dataRows, strikeCol, symbolOverride, defaultExpiry)
}

func parseWideCSV(header []string, dataRows [][]string, strikeCol int, sym string, defaultExpiry string) ([]models.OptionRecord, error) {
	if sym == "" {
		sym = "UNKNOWN"
	}
	if defaultExpiry == "" {
		defaultExpiry = "Current Expiry"
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
				Expiry: defaultExpiry,
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
				Expiry: defaultExpiry,
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

func parseTallCSV(header []string, dataRows [][]string, strikeCol int, symbolOverride string, defaultExpiry string) ([]models.OptionRecord, error) {
	sym, optType, expiry, oi, prevOI, vol, ltp, iv := -1, -1, -1, -1, -1, -1, -1, -1
	for i, h := range header {
		n := strings.ToLower(strings.TrimSpace(h))
		switch n {
		case "symbol", "scrip":
			sym = i
		case "option type", "type", "ce/pe", "optiontype":
			optType = i
		case "expiry", "expiry date", "expiry_date", "expirydate":
			expiry = i
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

	if defaultExpiry == "" {
		defaultExpiry = "Current Expiry"
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

		expiryVal := getCol(row, expiry)
		if expiryVal == "" {
			expiryVal = defaultExpiry
		}

		oiVal := cleanInt(getCol(row, oi))
		prev := maxInt(0, cleanInt(getCol(row, prevOI)))
		rec := models.OptionRecord{
			Symbol: s, StrikePrice: strike, OptionType: ot,
			Expiry: expiryVal,
			OI:     int64(oiVal), PrevOI: int64(prev),
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

// inferExpiryFromFilename extracts expiry date from common NSE download filenames.
// e.g. "option-chain-ED-RELIANCE-30-Jun-2026.csv" -> "30-Jun-2026"
func inferExpiryFromFilename(filename string) string {
	if filename == "" {
		return ""
	}
	base := strings.ToUpper(filepath.Base(filename))
	re := regexp.MustCompile(`OPTION-CHAIN-ED-[A-Z0-9]+-([0-9]{2}-[A-Z]{3}-[0-9]{4})`)
	if m := re.FindStringSubmatch(base); len(m) > 1 {
		parts := strings.Split(m[1], "-")
		if len(parts) == 3 {
			month := strings.Title(strings.ToLower(parts[1]))
			return fmt.Sprintf("%s-%s-%s", parts[0], month, parts[2])
		}
		return m[1]
	}
	return ""
}

// FetchNSE triggers a live fetch from NSE India, stores records and spot price, clears cached analysis.
func (h *Handler) FetchNSE(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if symbol == "" {
		jsonError(w, "query param 'symbol' required", http.StatusBadRequest)
		return
	}

	h.log.Info("fetching live NSE data", "symbol", symbol)
	records, spotPrice, err := nse.FetchLive(r.Context(), symbol)
	if err != nil {
		h.log.Error("NSE live fetch failed", "symbol", symbol, "error", err)
		jsonError(w, "NSE fetch failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.store.SaveRecords(r.Context(), symbol, records); err != nil {
		h.log.Error("failed to save records to store", "symbol", symbol, "error", err)
		jsonError(w, "save error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.store.SaveSpotPrice(r.Context(), symbol, spotPrice); err != nil {
		h.log.Warn("failed to save spot price to store", "symbol", symbol, "error", err)
	}

	_ = h.store.DeleteSignals(r.Context(), symbol)

	h.log.Info("NSE live fetch success", "symbol", symbol, "records", len(records), "spot", spotPrice)
	jsonOK(w, map[string]any{
		"symbol":     symbol,
		"records":    len(records),
		"spot_price": spotPrice,
		"message":    "Live options chain fetched successfully",
	})
}

// AIReport generates a single, consolidated AI commentary for a symbol based on its analyzed data.
func (h *Handler) AIReport(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if symbol == "" {
		jsonError(w, "query param 'symbol' required", http.StatusBadRequest)
		return
	}

	// Fetch cached analysis first
	analysis, err := h.store.GetAnalysis(r.Context(), symbol)
	if err != nil || analysis == nil {
		// Try to run analysis
		records, err := h.store.GetRecords(r.Context(), symbol)
		if err != nil || len(records) == 0 {
			jsonError(w, "no option data found for symbol "+symbol+". Upload or fetch data first.", http.StatusNotFound)
			return
		}
		spotPrice, _ := h.store.GetSpotPrice(r.Context(), symbol)
		signals := h.analyzer.Analyze(r.Context(), records)
		overall, expiries := core.CalcMetrics(records)
		analysis = &models.AnalyseResponse{
			Symbol:    symbol,
			Cached:    false,
			SpotPrice: spotPrice,
			Metrics:   overall,
			Expiries:  expiries,
			Signals:   signals,
		}
	}

	// If AI report already exists in cached analysis, return it!
	if analysis.AIReport != "" {
		jsonOK(w, map[string]string{"ai_report": analysis.AIReport})
		return
	}

	// Format top signals
	var sigSummary []string
	var sortedSignals []models.TradeSignal
	sortedSignals = append(sortedSignals, analysis.Signals...)
	sort.Slice(sortedSignals, func(i, j int) bool {
		actI := sortedSignals[i].Action
		actJ := sortedSignals[j].Action
		if (actI == "BUY" || actI == "SELL") && (actJ != "BUY" && actJ != "SELL") {
			return true
		}
		return false
	})

	limit := 5
	if len(sortedSignals) < limit {
		limit = len(sortedSignals)
	}
	for i := 0; i < limit; i++ {
		sig := sortedSignals[i]
		sigSummary = append(sigSummary, fmt.Sprintf("%s %0.f %s: %s (Confidence: %s)", sig.OptionType, sig.StrikePrice, sig.Action, sig.Rationale, sig.Confidence))
	}

	// Construct AI report prompt
	prompt := fmt.Sprintf(`You are an expert options trading strategist. Analyze this market snapshot and write a professional, action-oriented 1-2 paragraph market commentary. Include specific key support/resistance levels, a tactical trade strategy (e.g. credit/debit spread, naked selling warning, or watch/neutral), and risk warnings. Do not mention HTML tags.

Symbol: %s
Spot Price: %.2f
Overall PCR: %.2f (Put-Call Ratio)
Max Pain Strike: %.0f
IV Skew: %.2f (CE IV - PE IV)
Trade Bias: %s (Reason: %s)
Top CE Strikes (Resistance): %v
Top PE Strikes (Support): %v

Key Option Chain Signals:
- %s

Provide a structured, readable markdown report with headers.`,
		analysis.Symbol, analysis.SpotPrice, analysis.Metrics.PCR, analysis.Metrics.MaxPain,
		analysis.Metrics.IVSkew, analysis.Metrics.Bias, analysis.Metrics.BiasReason,
		analysis.Metrics.TopCEStrikes, analysis.Metrics.TopPEStrikes,
		strings.Join(sigSummary, "\n- "))

	// Call the analyzer's CompletePrompt to generate the report
	ctx := r.Context()
	report, err := h.analyzer.CompletePrompt(ctx, prompt)
	if err != nil {
		h.log.Warn("AI report generation failed, using rules-based report", "error", err)
		report = generateRulesReport(analysis)
	}

	analysis.AIReport = report
	// Save back to cache
	_ = h.store.SaveAnalysis(ctx, symbol, analysis)

	jsonOK(w, map[string]string{"ai_report": report})
}

func generateRulesReport(analysis *models.AnalyseResponse) string {
	var builder strings.Builder
	builder.WriteString("## Rules-Based Analyst Report *(LLM Offline)*\n\n")
	
	builder.WriteString(fmt.Sprintf("**Market Trend Bias:** The calculated sentiment bias is **%s**.\n\n", analysis.Metrics.Bias))
	builder.WriteString(fmt.Sprintf("%s\n\n", analysis.Metrics.BiasReason))
	
	builder.WriteString("### Key Support & Resistance Boundaries\n")
	builder.WriteString(fmt.Sprintf("- **Spot Price:** %.2f\n", analysis.SpotPrice))
	builder.WriteString(fmt.Sprintf("- **Max Pain (Expiry Anchor):** %.0f\n", analysis.Metrics.MaxPain))
	
	if len(analysis.Metrics.TopPEStrikes) > 0 {
		secondVal := 0.0
		if len(analysis.Metrics.TopPEStrikes) > 1 {
			secondVal = analysis.Metrics.TopPEStrikes[1]
		}
		builder.WriteString(fmt.Sprintf("- **Strong Floor Support (Put concentration):** %.0f (with secondary support at %.0f)\n", 
			analysis.Metrics.TopPEStrikes[0], secondVal))
	}
	if len(analysis.Metrics.TopCEStrikes) > 0 {
		secondVal := 0.0
		if len(analysis.Metrics.TopCEStrikes) > 1 {
			secondVal = analysis.Metrics.TopCEStrikes[1]
		}
		builder.WriteString(fmt.Sprintf("- **Strong Ceiling Resistance (Call concentration):** %.0f (with secondary resistance at %.0f)\n", 
			analysis.Metrics.TopCEStrikes[0], secondVal))
	}
	
	builder.WriteString("\n### Tactical Trading Guidance\n")
	switch analysis.Metrics.Bias {
	case "Bullish":
		builder.WriteString("- **Strategy Outlook:** Option writers are aggressively putting floors below the spot price. Put premiums are decaying fast due to heavy writing.\n")
		builder.WriteString(fmt.Sprintf("- **Action Plan:** Look for long entries or sell put spreads below key support levels (e.g. below the Max Pain level of %.0f).\n", analysis.Metrics.MaxPain))
	case "Bearish":
		builder.WriteString("- **Strategy Outlook:** Call writers have established high ceilings, indicating the market expects resistance on rallies. Spot price is under pressure.\n")
		builder.WriteString("- **Action Plan:** Consider hedged short positions or buying put spreads. Avoid selling naked calls due to outlier risks.\n")
	default:
		builder.WriteString(fmt.Sprintf("- **Strategy Outlook:** Total open interest is balanced between calls and puts. The price index is likely to pin close to the Max Pain strike of %.0f.\n", analysis.Metrics.MaxPain))
		builder.WriteString("- **Action Plan:** Sideways range-bound trading is expected. Consider selling credit spreads outside the support/resistance boundaries (e.g. Iron Condor strategy).\n")
	}
	
	builder.WriteString("\n*Disclaimer: This report is automatically generated based on static option chain mathematics and does not constitute financial advice.*")
	return builder.String()
}

