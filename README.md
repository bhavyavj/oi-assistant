# OI Assistant

AI-powered options Open Interest analysis tool. Upload an Excel file with OI data → get trade signals powered by an LLM (OpenAI or Ollama).

## What it does

1. Accepts an Excel file with options OI data (strike, OI, prev OI, volume, LTP)
2. Detects strikes where OI changed by more than 20%
3. Calls an LLM to generate a trade signal (BUY/SELL/WATCH) with rationale
4. Falls back to a deterministic rule-based engine if the LLM fails
5. Caches results in Redis (24h TTL) — repeated requests are instant

## Quick Start

### With OpenAI

```bash
export OPENAI_API_KEY=sk-...
export LLM_PROVIDER=openai
make docker-up
```

### With Ollama (local, no API key)

```bash
# Install Ollama: https://ollama.com
ollama pull llama3
export LLM_PROVIDER=ollama
make docker-up
```

### Generate sample data and test

```bash
# Install openpyxl once
pip install openpyxl

make sample    # creates scripts/sample_data/sample_options.xlsx
make upload    # POST the file
make analyse SYMBOL=NIFTY
```

## API

| Method | Path | Description |
|--------|------|-------------|
| POST | `/upload-excel` | Upload `.xlsx` file (`options_file` field) |
| GET | `/analyse?symbol=NIFTY` | Get trade signals for a symbol |
| GET | `/healthz` | Health check |

### Upload

```bash
curl -F "options_file=@data.xlsx" http://localhost:8080/upload-excel
```

### Analyse

```bash
curl "http://localhost:8080/analyse?symbol=NIFTY"
```

Response:
```json
{
  "symbol": "NIFTY",
  "cached": false,
  "signals": [
    {
      "symbol": "NIFTY",
      "strike_price": 22000,
      "option_type": "CE",
      "action": "BUY",
      "rationale": "Strong OI build-up of 85% suggests bullish momentum at this strike.",
      "confidence": "HIGH",
      "source": "llm"
    }
  ]
}
```

## Excel Format

| Column | Required | Accepted Names |
|--------|----------|----------------|
| Symbol | ✓ | symbol, scrip |
| Strike Price | ✓ | strike price, strike |
| Option Type | ✓ | option type, type, CE/PE |
| Expiry | | expiry, expiry date |
| OI | ✓ | oi, open interest |
| Prev OI | | prev oi, previous oi |
| Volume | | volume, vol |
| LTP | | ltp, last price |

## Architecture

```
cmd/server/main.go          entrypoint, wires everything together
internal/
  excel/parser.go           parses .xlsx with flexible header detection
  storage/redis_client.go   caches records and signals (24h TTL)
  llm/client.go             provider-agnostic interface (OpenAI + Ollama)
  core/analyzer.go          semaphore-bounded concurrent analysis + fallback
  api/handlers/handler.go   HTTP handlers
  api/routes.go             Chi router
  models/                   shared structs + config loader
pkg/logger/                 structured JSON logger (slog)
```

Key technical decisions:
- **Semaphore** (`chan struct{}`) limits concurrent LLM calls to 5 — prevents rate limit errors and controls cost
- **Provider interface** — swap OpenAI for Ollama by changing one env var, no code change
- **Output validation** — LLM response is parsed and validated; invalid output falls back to rule engine
- **Redis caching** — same symbol+data combo never hits the LLM twice within 24h

## Configuration

`configs/config.yaml` — all settings. Override via env:

| Env Var | Default | Description |
|---------|---------|-------------|
| `OPENAI_API_KEY` | | Required if provider=openai |
| `LLM_PROVIDER` | ollama | `openai` or `ollama` |
| `REDIS_ADDR` | redis:6379 | Redis address |
| `CONFIG_PATH` | configs/config.yaml | Config file path |

## License

MIT
