#!/usr/bin/env bash
# Starts the Go game server and the Vite dev server. Ctrl+C stops both.
set -euo pipefail
cd "$(dirname "$0")"
[ -f .env ] && set -a && . ./.env && set +a
(cd backend && go build -o bin/server ./cmd/server)
(cd frontend && [ -d node_modules ] || npm ci)
backend/bin/server &
BACK=$!
trap 'kill $BACK 2>/dev/null || true' EXIT
(cd frontend && npx vite)
