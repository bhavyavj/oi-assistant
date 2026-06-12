.PHONY: build run docker-up docker-down tidy test sample stop kill start upload-real analyse-real real start-real view summary

# Default symbol for make view / make analyse
SYMBOL ?= NIFTY

build:
	go build -ldflags="-s -w" -o bin/oi-assistant ./cmd/server

run: build
	CONFIG_PATH=configs/config.yaml ./bin/oi-assistant

docker-up:
	docker compose up --build

docker-down:
	docker compose down

tidy:
	go mod tidy

test:
	go test ./... -race -count=1

sample:
	python3 scripts/sample_data/generate_sample_excel.py

# Stop any running instance
stop kill:
	pkill -f oi-assistant || true
	@echo "Stopped oi-assistant (if it was running)"

# Start in background (recommended for normal use)
start: build
	CONFIG_PATH=configs/config.yaml nohup ./bin/oi-assistant > /tmp/oi-assistant.log 2>&1 &
	@echo "Server started in background on :8080"
	@echo "Logs: tail -f /tmp/oi-assistant.log"
	@sleep 1
	@curl -s http://localhost:8080/healthz | cat || true

# Upload the real data from your CSV (converted)
upload-real:
	curl -s -F "options_file=@scripts/sample_data/nifty_16jun2026.xlsx" \
		http://localhost:8080/upload-excel | cat

# Analyse using the real NIFTY data (pretty JSON)
analyse-real:
	curl -s "http://localhost:8080/analyse?symbol=NIFTY" | (jq . 2>/dev/null || python3 -m json.tool 2>/dev/null || cat)

# Human readable structured summary (recommended - easy to read)
# Usage: make view SYMBOL=BANKNIFTY   or just make view (defaults to NIFTY)
view summary:
	curl -s "http://localhost:8080/analyse?symbol=$(SYMBOL)" | python3 scripts/pretty_view.py $(SYMBOL)

# Original targets (kept for compatibility)
upload:
	curl -s -F "options_file=@scripts/sample_data/sample_options.xlsx" \
		http://localhost:8080/upload-excel | cat

analyse:
	curl -s "http://localhost:8080/analyse?symbol=$(SYMBOL)" | cat

health:
	curl -s http://localhost:8080/healthz | cat

# One-command experience — starts everything and prints the UI URL
real start-real:
	./run.sh

# Open the browser UI after starting the server
ui:
	@echo "Open http://localhost:8080 in your browser"
	@echo "Drag & drop your CSV, enter symbol, analyze."
