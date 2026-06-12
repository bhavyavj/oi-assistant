#!/usr/bin/env bash
set -e

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_DIR"

PID_FILE=/tmp/oi-assistant.pid
LOG_FILE=/tmp/oi-assistant.log

# Redis
if ! redis-cli ping &>/dev/null; then
  echo "Starting Redis..."
  brew services start redis
  sleep 1
fi

# Stop previous instance cleanly
if [ -f "$PID_FILE" ]; then
  OLD_PID=$(cat "$PID_FILE")
  if kill -0 "$OLD_PID" 2>/dev/null; then
    kill "$OLD_PID" && sleep 1
  fi
  rm -f "$PID_FILE"
fi

# Build
go build -o bin/oi-assistant ./cmd/server/

# Start
./bin/oi-assistant > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"

# Wait for health
for i in $(seq 1 10); do
  curl -sf http://localhost:8080/healthz &>/dev/null && break
  sleep 1
done

# Upload file if provided: ./run.sh ~/Downloads/file.csv SYMBOL
if [ $# -ge 2 ] && [ -f "$1" ]; then
  echo "Uploading $1..."
  curl -sS -F "options_file=@$1" -F "symbol=$2" http://localhost:8080/upload-excel
  echo ""
fi

echo "Ready → http://localhost:8080  (logs: tail -f $LOG_FILE)"
