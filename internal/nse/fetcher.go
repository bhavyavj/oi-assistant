package nse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/bhavyavj/oi-assistant/internal/models"
)

type NseResponse struct {
	Records struct {
		ExpiryDates     []string  `json:"expiryDates"`
		UnderlyingValue float64   `json:"underlyingValue"`
		Data            []NseData `json:"data"`
	} `json:"records"`
}

type NseData struct {
	StrikePrice float64  `json:"strikePrice"`
	ExpiryDate  string   `json:"expiryDate"`
	CE          *NseInfo `json:"CE,omitempty"`
	PE          *NseInfo `json:"PE,omitempty"`
}

type NseInfo struct {
	StrikePrice          float64 `json:"strikePrice"`
	ExpiryDate           string  `json:"expiryDate"`
	Underlying           string  `json:"underlying"`
	OpenInterest         float64 `json:"openInterest"`
	ChangeInOpenInterest float64 `json:"changeinOpenInterest"`
	TotalTradedVolume    float64 `json:"totalTradedVolume"`
	LastPrice            float64 `json:"lastPrice"`
	ImpliedVolatility    float64 `json:"impliedVolatility"`
}

func isIndex(symbol string) bool {
	indices := map[string]bool{
		"NIFTY":      true,
		"BANKNIFTY":  true,
		"FINNIFTY":   true,
		"MIDCPNIFTY": true,
		"NIFTYNXT50": true,
	}
	return indices[strings.ToUpper(symbol)]
}

// FetchLive queries NSE India for the option chain, handles session cookies, and parses it.
func FetchLive(ctx context.Context, symbol string) ([]models.OptionRecord, float64, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return nil, 0, fmt.Errorf("empty symbol")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, 0, fmt.Errorf("cookie jar creation failed: %w", err)
	}

	client := &http.Client{
		Jar:     jar,
		Timeout: 15 * time.Second,
	}

	// Step 1: Visit home page to get session cookies
	reqHome, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.nseindia.com", nil)
	if err != nil {
		return nil, 0, err
	}
	setHeaders(reqHome)

	respHome, err := client.Do(reqHome)
	if err != nil {
		return nil, 0, fmt.Errorf("failed visiting NSE homepage: %w", err)
	}
	respHome.Body.Close()

	// Step 2: Fetch options chain API
	var apiURL string
	if isIndex(symbol) {
		apiURL = "https://www.nseindia.com/api/option-chain-indices?symbol=" + symbol
	} else {
		// Equities requires URL-encoding or just direct symbol if safe
		apiURL = "https://www.nseindia.com/api/option-chain-equities?symbol=" + symbol
	}

	reqAPI, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, 0, err
	}
	setHeaders(reqAPI)
	reqAPI.Header.Set("Referer", "https://www.nseindia.com/option-chain")

	respAPI, err := client.Do(reqAPI)
	if err != nil {
		return nil, 0, fmt.Errorf("failed calling NSE API: %w", err)
	}
	defer respAPI.Body.Close()

	if respAPI.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("NSE API returned status %d", respAPI.StatusCode)
	}

	var nseResp NseResponse
	if err := json.NewDecoder(respAPI.Body).Decode(&nseResp); err != nil {
		return nil, 0, fmt.Errorf("failed decoding NSE response: %w", err)
	}

	var records []models.OptionRecord
	for _, d := range nseResp.Records.Data {
		// Process CE if available
		if d.CE != nil && d.CE.OpenInterest > 0 {
			oi := int64(d.CE.OpenInterest)
			chg := int64(d.CE.ChangeInOpenInterest)
			prev := oi - chg
			if prev < 0 {
				prev = 0
			}
			rec := models.OptionRecord{
				Symbol:      symbol,
				StrikePrice: d.StrikePrice,
				OptionType:  "CE",
				Expiry:      d.ExpiryDate,
				OI:          oi,
				PrevOI:      prev,
				Volume:      int64(d.CE.TotalTradedVolume),
				LTP:         d.CE.LastPrice,
				IV:          d.CE.ImpliedVolatility,
			}
			if rec.PrevOI > 0 {
				rec.OIChange = float64(rec.OI-rec.PrevOI) / float64(rec.PrevOI) * 100
			}
			records = append(records, rec)
		}
		// Process PE if available
		if d.PE != nil && d.PE.OpenInterest > 0 {
			oi := int64(d.PE.OpenInterest)
			chg := int64(d.PE.ChangeInOpenInterest)
			prev := oi - chg
			if prev < 0 {
				prev = 0
			}
			rec := models.OptionRecord{
				Symbol:      symbol,
				StrikePrice: d.StrikePrice,
				OptionType:  "PE",
				Expiry:      d.ExpiryDate,
				OI:          oi,
				PrevOI:      prev,
				Volume:      int64(d.PE.TotalTradedVolume),
				LTP:         d.PE.LastPrice,
				IV:          d.PE.ImpliedVolatility,
			}
			if rec.PrevOI > 0 {
				rec.OIChange = float64(rec.OI-rec.PrevOI) / float64(rec.PrevOI) * 100
			}
			records = append(records, rec)
		}
	}

	if len(records) == 0 {
		return nil, 0, fmt.Errorf("no option data found for symbol %s (NSE may have blocked the request)", symbol)
	}

	return records, nseResp.Records.UnderlyingValue, nil
}

func setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
}
