package models

// OptionRecord represents one row from the uploaded CSV/Excel file.
type OptionRecord struct {
	Symbol      string  `json:"symbol"`
	StrikePrice float64 `json:"strike_price"`
	OptionType  string  `json:"option_type"` // CE or PE
	Expiry      string  `json:"expiry"`
	OI          int64   `json:"oi"`
	PrevOI      int64   `json:"prev_oi"`
	OIChange    float64 `json:"oi_change_pct"` // computed
	Volume      int64   `json:"volume"`
	LTP         float64 `json:"ltp"`
	IV          float64 `json:"iv"` // implied volatility (from broker CSV)
}

// OIMetrics holds aggregate metrics for a single expiry (or all expiries combined).
type OIMetrics struct {
	Expiry      string    `json:"expiry,omitempty"`
	PCR         float64   `json:"pcr"`
	MaxPain     float64   `json:"max_pain"`
	IVSkew      float64   `json:"iv_skew"`       // avg CE IV - avg PE IV; positive = calls pricier (bearish tilt)
	Bias        string    `json:"bias"`          // "Bullish" | "Bearish" | "Neutral"
	BiasReason  string    `json:"bias_reason"`
	TopCEStrikes []float64 `json:"top_ce_strikes"`
	TopPEStrikes []float64 `json:"top_pe_strikes"`
	TotalCEOI   int64     `json:"total_ce_oi"`
	TotalPEOI   int64     `json:"total_pe_oi"`
}

// TradeSignal is the output of the analyzer for a single record.
type TradeSignal struct {
	Symbol      string  `json:"symbol"`
	StrikePrice float64 `json:"strike_price"`
	OptionType  string  `json:"option_type"`
	Expiry      string  `json:"expiry"`
	Action      string  `json:"action"`     // BUY / SELL / WATCH
	Rationale   string  `json:"rationale"`
	Confidence  string  `json:"confidence"` // HIGH / MEDIUM / LOW
	Source      string  `json:"source"`     // "llm" or "fallback"
}

// AnalyseResponse is the full response from /analyse.
type AnalyseResponse struct {
	Symbol    string         `json:"symbol"`
	Cached    bool           `json:"cached"`
	SpotPrice float64        `json:"spot_price,omitempty"`
	AIReport  string         `json:"ai_report,omitempty"`
	Metrics   OIMetrics      `json:"metrics"`           // overall (all expiries combined)
	Expiries  []OIMetrics    `json:"expiries"`          // per-expiry breakdown
	Signals   []TradeSignal  `json:"signals"`
	Records   []OptionRecord `json:"records,omitempty"`
}
