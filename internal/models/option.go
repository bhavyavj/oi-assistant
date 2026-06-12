package models

// OptionRecord represents one row from the uploaded Excel file.
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
}

// TradeSignal is the output of the analyzer.
type TradeSignal struct {
	Symbol      string  `json:"symbol"`
	StrikePrice float64 `json:"strike_price"`
	OptionType  string  `json:"option_type"`
	Action      string  `json:"action"`      // BUY / SELL / WATCH
	Rationale   string  `json:"rationale"`
	Confidence  string  `json:"confidence"`  // HIGH / MEDIUM / LOW
	Source      string  `json:"source"`      // "llm" or "fallback"
}
