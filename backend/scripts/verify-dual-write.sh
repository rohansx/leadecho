#!/usr/bin/env bash
# Phase 1 dual-write verification (local or staging).
# Requires: Docker Postgres + Redis, Go 1.25+, curl, redis-cli (optional).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DATABASE_URL="${DATABASE_URL:-postgres://leadecho:leadecho@localhost:5433/leadecho_dev?sslmode=disable}"
REDIS_URL="${REDIS_URL:-redis://localhost:6380/0}"
API_URL="${API_URL:-http://localhost:8090}"
PORT="${PORT:-8090}"

export DATABASE_URL REDIS_URL
export ENVIRONMENT="${ENVIRONMENT:-development}"
export JWT_SECRET="${JWT_SECRET:-leadecho-dev-secret-change-in-prod}"

# Phase 1: publish + inline, no consumers
export STREAMS_ENABLED=true
export STREAMS_DUAL_WRITE_ENABLED=true
export STREAMS_INLINE_FALLBACK_ENABLED=true
export STREAMS_WORKERS_IN_API=false
export STREAMS_SCORER_CONSUMER_ENABLED=false
export STREAMS_NOTIFIER_CONSUMER_ENABLED=false
export STREAMS_QUALIFIER_CONSUMER_ENABLED=false
export STREAMS_REPLY_DRAFTER_CONSUMER_ENABLED=false
export STREAMS_WORKFLOW_CONSUMER_ENABLED=false
export STREAMS_RETRY_CONSUMER_ENABLED=false
export METRICS_ENABLED=true

echo "==> Applying migrations (including streams repair if needed)"
go run github.com/pressly/goose/v3/cmd/goose@latest -dir migrations postgres "$DATABASE_URL" up

echo "==> Running integration tests (dual-write + full pipeline)"
go test -tags=integration -timeout=5m ./internal/integration/...

echo "==> Building API"
go build -o /tmp/leadecho-api ./cmd/api

if curl -sf "$API_URL/healthz" >/dev/null 2>&1; then
  echo "==> API already running at $API_URL — skipping start"
else
  echo "==> Starting API on :$PORT (dual-write Phase 1 env)"
  /tmp/leadecho-api &
  API_PID=$!
  trap 'kill $API_PID 2>/dev/null || true' EXIT

  for i in $(seq 1 30); do
    if curl -sf "$API_URL/healthz" >/dev/null; then
      break
    fi
    sleep 1
  done
  if ! curl -sf "$API_URL/healthz" >/dev/null; then
    echo "API failed to start"
    exit 1
  fi
fi

echo "==> Checking /metrics"
curl -sf "$API_URL/metrics" | head -5 || echo "(metrics endpoint optional if METRICS_ENABLED=false)"

echo "==> Postgres event_log snapshot"
psql "$DATABASE_URL" -c "SELECT event_type, publish_status, COUNT(*) FROM event_log GROUP BY 1,2 ORDER BY 1;" 2>/dev/null || \
  echo "Install psql or query event_log manually"

REDIS_HOST="${REDIS_URL#redis://}"
REDIS_HOST="${REDIS_HOST%%/*}"
REDIS_PORT="${REDIS_HOST##*:}"
REDIS_HOST="${REDIS_HOST%%:*}"
if command -v redis-cli >/dev/null; then
  echo "==> Redis stream lengths"
  redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6380}" XLEN leadecho:mention_events || true
  redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6380}" XLEN leadecho:reply_events || true
fi

echo ""
echo "Phase 1 dual-write verification complete."
echo "Next: enable consumers one-by-one (Phase 2) and compare inline vs async outcomes."
