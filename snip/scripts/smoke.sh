#!/usr/bin/env bash
# smoke.sh — prove a running snip can create a link and redirect to it.
# Usage: SNIP_API_KEY=snip_... ./smoke.sh [BASE_URL]
set -euo pipefail

BASE_URL="${1:-http://localhost:8080}"
: "${SNIP_API_KEY:?set SNIP_API_KEY to a key from 'snip keys create'}"

log()  { echo "[smoke] $*"; }
fail() { echo "[smoke] FAIL: $*" >&2; exit 1; }   # >&2 sends the message to stderr

command -v curl >/dev/null || fail "curl is not installed"

# 1. Wait for the service to be healthy (up to 30 seconds).
for attempt in $(seq 1 30); do
  if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then break; fi
  [[ "$attempt" -eq 30 ]] && fail "$BASE_URL/healthz not healthy after 30s"
  sleep 1
done
log "healthy"

# 2. Create a link with a unique slug.
slug="smoke-$(date +%s)"
status=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/links" \
  -H "Authorization: Bearer $SNIP_API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"https://example.com/smoke\",\"slug\":\"$slug\"}")
[[ "$status" == "201" ]] || fail "create returned HTTP $status, expected 201"
log "created /$slug"

# 3. Follow it: expect a 302 pointing at the right place.
location=$(curl -s -o /dev/null -w "%{redirect_url}" "$BASE_URL/$slug")
[[ "$location" == "https://example.com/smoke" ]] || fail "redirect went to '$location'"
log "redirect ok"

# 4. Clean up after ourselves.
curl -fsS -X DELETE "$BASE_URL/api/links/$slug" -H "Authorization: Bearer $SNIP_API_KEY" >/dev/null
log "PASS"
