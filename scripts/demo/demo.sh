#!/usr/bin/env bash
set -euo pipefail

# Load environment variables if present
ENV_FILE="$(dirname "$0")/../../.env"
if [ -f "$ENV_FILE" ]; then
  source "$ENV_FILE"
fi

# Derive REDIS_HOST and REDIS_PORT from REDIS_ADDR if set
if [ -n "${REDIS_ADDR:-}" ]; then
  REDIS_HOST="${REDIS_ADDR%:*}"
  REDIS_PORT="${REDIS_ADDR#*:}"
fi


API="${API_URL:-http://localhost:8080}"
ADMIN_API="${ADMIN_URL:-http://localhost:8081}"
USER_ID="${DEMO_USER:-22222222-2222-2222-2222-222222222222}"
DOOR="${DEMO_DOOR:-11111111-1111-1111-1111-111111111111}"
BANNED_ID="${BANNED_USER:-00000000-0000-0000-0000-000000000099}"
BAN_WAIT="${BAN_WAIT:-3}"

swipe() {
  local user="$1" dir="$2"
  echo ">>> Swipe ${dir} (user=${user})"
  curl -skL -H "X-API-Key: ${API_KEY:-dev-api-key-2026}" -X POST "${API}/access/swipe" \
    -H "Content-Type: application/json" \
    -d "{\"userId\":\"${user}\",\"doorId\":\"${DOOR}\",\"direction\":\"${dir}\",\"cardUid\":\"CARD001\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" | jq .
  echo
}

echo "=== Access Fast Path Demo ==="
echo
# Clear passback state beforehand so the run is reproducible
if command -v redis-cli >/dev/null 2>&1; then
  redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" DEL "passback:${USER_ID}" >/dev/null || true
else
  docker compose exec -T redis redis-cli DEL "passback:${USER_ID}" >/dev/null 2>&1 || true
fi

swipe "$USER_ID" "IN"
echo "Expected: ALLOW"
echo

swipe "$USER_ID" "IN"
echo "Expected: DENY (ANTI_PASSBACK)"
echo

swipe "$USER_ID" "OUT"
echo "Expected: ALLOW"
echo

echo ">>> Ban user via Admin API (then swipe)"
curl -sfkL -H "X-API-Key: ${API_KEY:-dev-api-key-2026}" -X POST "${ADMIN_API}/admin/employees/${BANNED_ID}/ban" | jq .
sleep "$BAN_WAIT"
swipe "$BANNED_ID" "IN"
echo "Expected: DENY (PERMISSION_DENIED)"
echo

echo ">>> Employee state"
curl -skL -H "X-API-Key: ${API_KEY:-dev-api-key-2026}" "${API}/access/employee/${USER_ID}/state" | jq .
echo

echo ">>> Door status"
curl -skL -H "X-API-Key: ${API_KEY:-dev-api-key-2026}" "${API}/access/door/${DOOR}/status" | jq .
