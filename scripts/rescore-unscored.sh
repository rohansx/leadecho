#!/usr/bin/env bash
# Re-score mentions that were crawled before an AI provider was configured.
#
# The crawler only scores mentions it inserts in that same tick: insertMention()
# returns nil for duplicates, so batchScoreMentions() never sees pre-existing
# rows and re-crawling does nothing for them. This walks the per-mention
# classify endpoint instead.
#
# Usage:
#   ./scripts/rescore-unscored.sh <email> <password> [limit]
#
# Each mention costs one LLM call against your configured provider key.

set -euo pipefail

API="${LEADECHO_API:-http://localhost:8090}"
EMAIL="${1:?usage: rescore-unscored.sh <email> <password> [limit]}"
PASSWORD="${2:?usage: rescore-unscored.sh <email> <password> [limit]}"
LIMIT="${3:-25}"

COOKIE_JAR="$(mktemp)"
trap 'rm -f "$COOKIE_JAR"' EXIT

echo "Signing in as $EMAIL ..."
code=$(curl -s -o /dev/null -w '%{http_code}' -c "$COOKIE_JAR" \
  -X POST "$API/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
if [ "$code" != "200" ]; then
  echo "login failed (HTTP $code)" >&2
  exit 1
fi

echo "Fetching unscored mentions (limit $LIMIT) ..."
ids=$(curl -s -b "$COOKIE_JAR" "$API/api/v1/mentions?limit=500" \
  | python3 -c "
import json,sys
data = json.load(sys.stdin)
rows = data['data'] if isinstance(data, dict) else data
out = [m['id'] for m in rows if isinstance(m, dict) and not m.get('intent')]
print('\n'.join(out[:$LIMIT]))
")

total=$(printf '%s\n' "$ids" | grep -c . || true)
if [ "$total" -eq 0 ]; then
  echo "Nothing to do — no unscored mentions."
  exit 0
fi

echo "Classifying $total mentions ..."
n=0
while IFS= read -r id; do
  [ -z "$id" ] && continue
  n=$((n + 1))
  status=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE_JAR" --max-time 120 \
    -X POST "$API/api/v1/mentions/$id/classify")
  printf '  [%d/%d] %s -> %s\n' "$n" "$total" "${id:0:8}" "$status"
  sleep 0.5   # be gentle with provider rate limits
done <<< "$ids"

echo "Done. Open the inbox and switch to the Action required queue."
