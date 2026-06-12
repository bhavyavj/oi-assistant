# OI Assistant

**AI-powered (or rule-based) Open Interest analysis for options trading.**

Upload a broker-downloaded CSV (or XLSX) → instantly see structured **BUY / SELL / WATCH** signals with bias summary and actionable insights.

- Direct support for raw **wide-format broker CSVs** (no manual conversion required)
- Works for **NIFTY, BANKNIFTY, individual stocks** (KALYANKJIL, RELIANCE, etc.)
- Clean web UI at `http://localhost:8080`
- Local-first (no data leaves your machine unless you use OpenAI)
- 24h Redis caching for repeated analysis

---

## Quick Start (Recommended for most users)

### 1. One-command start (native)

```bash
git clone https://github.com/yourname/oi-assistant.git
cd oi-assistant

# Install Redis if you don't have it (macOS example)
# brew install redis && brew services start redis

./run.sh
```

The script will:
- Check/start Redis
- Build the Go server
- Start it in the background
- Print the UI URL

**Open http://localhost:8080** in your browser.

### 2. Using the UI

1. Download an option chain CSV from your broker (NSE / any platform that exports the classic CALLS/PUTS layout).
2. In the UI:
   - Enter the **Symbol** (e.g. `KALYANKJIL`, `NIFTY`, `RELIANCE`)
   - Drag & drop or select your CSV
   - Click **Upload & Process**
3. The symbol field on the right will auto-fill. Click **Analyze**.
4. You get:
   - Counts (BUY / SELL / WATCH)
   - Clear **Trade Bias & Conclusion**
   - Clean table of every signal with rationale

### Alternative one-liner (after first run)

```bash
cd oi-assistant
./run.sh ~/Downloads/option-chain-ED-KALYANKJIL-30-Jun-2026.csv KALYANKJIL
```

Then open http://localhost:8080 and click **Analyze** (or use the CLI below).

---

## Alternative Ways to Run

### Using Make (fastest after first setup)

```bash
make start          # starts server in background + prints UI URL
make view           # nice CLI summary for NIFTY
make view SYMBOL=KALYANKJIL
```

### Using Docker (best for third-party users / other OS)

```bash
export LLM_PROVIDER=ollama   # or openai
make docker-up
```

Then open http://localhost:8080.

### Manual (if you prefer)

```bash
# 1. Redis must be running on localhost:6379
redis-cli ping

# 2. Start the server
go run cmd/server/main.go
# or
make build && ./bin/oi-assistant
```

---

## Analyzing Your Own Downloads (Any Stock)

The system now understands the raw wide-format CSVs you download directly.

**CLI way (recommended for power users):**
```bash
./run.sh ~/Downloads/option-chain-ED-RELIANCE-....csv RELIANCE
curl "http://localhost:8080/analyse?symbol=RELIANCE" | python3 scripts/pretty_view.py RELIANCE
```

**UI way:** Just use the upload form in the browser (as described above).

**Direct curl (for scripts):**
```bash
curl -F "options_file=@~/Downloads/your-file.csv" \
     -F "symbol=RELIANCE" \
     http://localhost:8080/upload-excel

curl "http://localhost:8080/analyse?symbol=RELIANCE" | python3 scripts/pretty_view.py RELIANCE
```

**Even better (but experimental): Fetch live directly from NSE India**

In the browser UI you will see a **"Fetch & Analyze"** button under "Or fetch live from NSE India".

It tries to pull the latest option chain data directly from NSE's API (no manual download).

**Via API:**
```bash
curl "http://localhost:8080/fetch-nse?symbol=NIFTY"
curl "http://localhost:8080/fetch-nse?symbol=KALYANKJIL"
curl "http://localhost:8080/fetch-nse?symbol=RELIANCE"
```

**Important limitations (for GitHub users):**
- NSE is very aggressive with anti-bot protection.
- Direct fetch often returns 404 / "Resource not found" or gets blocked.
- When it fails, the UI shows a clear message with a link to manually download the CSV.
- **Reliable method (always works):** Download the CSV manually from https://www.nseindia.com/option-chain → use the "Upload & Process" section in the UI (or `./run.sh yourfile.csv SYMBOL`).

We recommend using the direct fetch when it works, and falling back to CSV upload otherwise. The rest of the project (analysis, UI, bias summary) works identically either way.

---

## What You Get in the Output

- **Total significant moves** (>20% OI change)
- Separate BUY (bullish CE) and SELL (bearish PE) lists with confidence
- **Trade Bias & Conclusion** section (auto-generated from the data)
- Full table of signals

The backend falls back to deterministic rules if no LLM is configured. You can enable real LLM analysis by setting `OPENAI_API_KEY`.

---

## Configuration & LLM

Create a `.env` or export:

```bash
export OPENAI_API_KEY=sk-...
export LLM_PROVIDER=openai     # or "ollama"
```

Default is `ollama` (expects Ollama running locally on port 11434).

---

## Project Structure (for contributors)

```
cmd/server/main.go          # entrypoint
internal/
  api/handlers/handler.go   # upload + analyse logic (+ CSV support)
  excel/parser.go           # Excel + wide CSV parser
  core/analyzer.go          # signal generation + fallback
web/static/index.html       # self-contained browser UI
scripts/
  pretty_view.py            # beautiful CLI/structured output
run.sh                      # the magic one-command script
Makefile                    # make start, make view, make real, etc.
```

---

## Publishing / For Third-Party Users

1. Clone the repo
2. Make sure you have Go 1.22+ and Redis (or use Docker)
3. Run `./run.sh`
4. Open browser → done.

Everything a stranger needs is in the README + the `run.sh` script + the browser UI.

---

## Roadmap Ideas (contributions welcome)

- Auto-detect latest CSV from `~/Downloads`
- Support for more CSV layouts
- Historical comparison between uploads
- Export to CSV / image
- Better LLM prompt with more context

---

## License

MIT

---

**Made for people who actually download option chain CSVs every day.**

If you find it useful, star the repo and drop your feedback!