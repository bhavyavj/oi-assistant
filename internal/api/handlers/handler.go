package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bhavyavj/oi-assistant/internal/core"
	"github.com/bhavyavj/oi-assistant/internal/excel"
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
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB limit
		jsonError(w, "file too large (max 10MB)", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("options_file")
	if err != nil {
		jsonError(w, "field 'options_file' required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".xlsx" && ext != ".xls" {
		jsonError(w, "only .xlsx or .xls files accepted", http.StatusBadRequest)
		return
	}

	// Write to temp file (excelize needs a file path)
	tmp, err := os.CreateTemp("", "oi-*.xlsx")
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

	records, err := excel.Parse(tmp.Name())
	if err != nil {
		jsonError(w, "parse error: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	// Group by symbol and cache
	bySymbol := make(map[string]int)
	for _, rec := range records {
		bySymbol[rec.Symbol]++
		if err := h.store.SaveRecords(r.Context(), rec.Symbol, records); err != nil {
			h.log.Error("redis save failed", "error", err)
		}
	}

	h.log.Info("upload processed", "records", len(records), "symbols", len(bySymbol))

	jsonOK(w, map[string]any{
		"records":  len(records),
		"symbols":  keys(bySymbol),
		"message":  "upload successful, call /analyse?symbol=SYMBOL to get signals",
	})
}

// Analyse retrieves cached OI data for a symbol and runs the analyzer.
func (h *Handler) Analyse(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(r.URL.Query().Get("symbol"))
	if symbol == "" {
		jsonError(w, "query param 'symbol' required", http.StatusBadRequest)
		return
	}

	// Check cached signals first
	cached, err := h.store.GetSignals(r.Context(), symbol)
	if err == nil && len(cached) > 0 {
		jsonOK(w, map[string]any{"symbol": symbol, "signals": cached, "cached": true})
		return
	}

	records, err := h.store.GetRecords(r.Context(), symbol)
	if err != nil {
		jsonError(w, "redis error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(records) == 0 {
		jsonError(w, "no data for symbol "+symbol+"; upload an Excel file first", http.StatusNotFound)
		return
	}

	signals := h.analyzer.Analyze(r.Context(), records)
	if len(signals) == 0 {
		jsonOK(w, map[string]any{
			"symbol":  symbol,
			"signals": []any{},
			"message": "no significant OI changes detected",
		})
		return
	}

	// Cache results
	if err := h.store.SaveSignals(r.Context(), symbol, signals); err != nil {
		h.log.Warn("failed to cache signals", "error", err)
	}

	jsonOK(w, map[string]any{"symbol": symbol, "signals": signals, "cached": false})
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
