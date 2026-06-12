#!/usr/bin/env bash
set -e

echo "==> Checking Redis..."
if ! command -v redis-cli &>/dev/null; then
  brew install redis
fi
if ! redis-cli ping &>/dev/null 2>&1; then
  brew services start redis
  sleep 1
fi
echo "    Redis OK"

echo "==> Generating sample Excel data..."
VENV=/tmp/oi-venv
python3 -m venv "$VENV"
"$VENV/bin/pip" install -q openpyxl
"$VENV/bin/python3" scripts/sample_data/generate_sample_excel.py

echo "==> Patching config for localhost Redis..."
sed -i '' 's|redis:6379|localhost:6379|' configs/config.yaml

echo "==> Building server..."
make build

echo "==> Freeing port 8080..."
lsof -ti :8080 | xargs kill -9 2>/dev/null || true
sleep 1

echo "==> Starting server..."
./bin/oi-assistant &
SERVER_PID=$!
sleep 2

echo "==> Uploading sample data..."
curl -s -F "options_file=@scripts/sample_data/sample_options.xlsx" \
  http://localhost:8080/upload-excel

echo ""
echo "==> Getting trade signals for NIFTY..."
curl -s "http://localhost:8080/analyse?symbol=NIFTY"

echo ""
echo ""
echo "Done! Server PID: $SERVER_PID"
echo "  curl 'http://localhost:8080/analyse?symbol=BANKNIFTY'"
echo "  kill $SERVER_PID   (to stop)"
