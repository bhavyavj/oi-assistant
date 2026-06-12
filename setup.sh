#!/usr/bin/env bash
set -e

echo "==> Checking Redis..."
if ! command -v redis-cli &>/dev/null; then
  echo "    Installing Redis via Homebrew..."
  brew install redis
fi
if ! redis-cli ping &>/dev/null 2>&1; then
  echo "    Starting Redis..."
  brew services start redis
  sleep 1
fi
echo "    Redis OK"

echo "==> Generating sample Excel data..."
VENV=/tmp/oi-venv
python3 -m venv "$VENV"
"$VENV/bin/pip" install -q openpyxl
"$VENV/bin/python3" scripts/sample_data/generate_sample_excel.py

echo "==> Setting config to use localhost Redis..."
sed -i '' 's|redis:6379|localhost:6379|' configs/config.yaml

echo "==> Building server..."
make build

echo "==> Starting server in background..."
./bin/oi-assistant &
SERVER_PID=$!
echo "    Server PID: $SERVER_PID"
sleep 2

echo "==> Uploading sample data..."
curl -s -F "options_file=@scripts/sample_data/sample_options.xlsx" \
  http://localhost:8080/upload-excel | python3 -m json.tool

echo ""
echo "==> Getting trade signals for NIFTY..."
curl -s "http://localhost:8080/analyse?symbol=NIFTY" | python3 -m json.tool

echo ""
echo "Done! Server is running (PID $SERVER_PID)."
echo "  Analyse any symbol: curl 'http://localhost:8080/analyse?symbol=BANKNIFTY'"
echo "  Stop server:        kill $SERVER_PID"
