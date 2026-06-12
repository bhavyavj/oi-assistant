.PHONY: build run docker-up docker-down tidy test sample

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

upload:
	curl -s -F "options_file=@scripts/sample_data/sample_options.xlsx" \
		http://localhost:8080/upload-excel | jq .

analyse:
	curl -s "http://localhost:8080/analyse?symbol=$(SYMBOL)" | jq .

health:
	curl -s http://localhost:8080/healthz | jq .
