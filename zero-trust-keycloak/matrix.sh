#!/usr/bin/env bash
# Runs the acceptance matrix of "Keycloak Is Not Your Zero Trust Architecture"
# against the lab's inventory-api and payment-api.
# Requires: curl, jq, Docker Compose, and the provisioned lab (see README.md).
set -euo pipefail
cd "$(dirname "$0")"

KC_URL="${KC_URL:-http://localhost:8080}"
INVENTORY_URL="${INVENTORY_URL:-http://localhost:8081}"
PAYMENT_URL="${PAYMENT_URL:-http://localhost:8082}"
DEMO_TOKEN_URL="$KC_URL/realms/demo/protocol/openid-connect/token"
PARTNER_TOKEN_URL="$KC_URL/realms/partner/protocol/openid-connect/token"
TOKEN_EXCHANGE="urn:ietf:params:oauth:grant-type:token-exchange"
ACCESS_TOKEN_TYPE="urn:ietf:params:oauth:token-type:access_token"
LEEWAY_SECONDS=5 # the APIs' clock-skew allowance, LEEWAY in docker-compose.yml

RESPONSE_BODY=$(mktemp)
trap 'rm -f "$RESPONSE_BODY"' EXIT

# Print the access token from a token endpoint response, or stop with the error.
access_token() {
  local token
  token=$(jq -r '.access_token // empty' <<<"$1")
  if [[ -z $token ]]; then
    echo "Keycloak returned no access token: $1" >&2
    exit 1
  fi
  printf '%s' "$token"
}

# Log in through shop-web with the password grant (lab only). Args: username, scope.
login() {
  access_token "$(curl -sS "$DEMO_TOKEN_URL" -d client_id=shop-web -d grant_type=password \
    -d username="$1" -d password="$1" --data-urlencode scope="$2")"
}

# order-api exchanges a subject token. Extra args are passed to curl.
exchange() {
  local subject_token=$1
  shift
  access_token "$(curl -sS "$DEMO_TOKEN_URL" -u order-api:order-api-secret \
    --data-urlencode grant_type="$TOKEN_EXCHANGE" \
    --data-urlencode subject_token_type="$ACCESS_TOKEN_TYPE" \
    -d subject_token="$subject_token" "$@")"
}

# Print the payload of a JWT (base64url-decoded, padding restored).
payload() {
  local part
  part=$(cut -d. -f2 <<<"$1" | tr '_-' '/+')
  case $(( ${#part} % 4 )) in
    2) part="$part==" ;;
    3) part="$part=" ;;
  esac
  base64 -d <<<"$part"
}

show_claims() {
  printf '\n%s\n' "$1"
  payload "$2" | jq '{iss, azp, aud, preferred_username, resource_access, iat, exp}'
}

FAILURES=0

# Call an API and print one row of the matrix.
# Args: number, description, expected status, method, URL, token ("" for none), JSON body (optional).
check() {
  local number=$1 description=$2 expected=$3 method=$4 url=$5 token=$6 body=${7:-}
  local args=(-sS -X "$method" -o "$RESPONSE_BODY" -w '%{http_code}')
  [[ -n $token ]] && args+=(-H "Authorization: Bearer $token")
  [[ -n $body ]] && args+=(-H 'Content-Type: application/json' -d "$body")

  local status detail result=ok
  status=$(curl "${args[@]}" "$url")
  detail=$(jq -r '.error_description
    // (if .payment_id then "\(.payment_id): \(.currency) \(.amount)" else empty end)
    // (if .sku then "\(.sku): \(.available) in stock" else empty end)
    // tostring' "$RESPONSE_BODY")
  if [[ $status != "$expected" ]]; then
    result=FAIL
    FAILURES=$((FAILURES + 1))
  fi
  printf '%3s  %-46s %4s %4s  %s\n       %s\n' "$number" "$description" "$expected" "$status" "$result" "$detail"
}

for _ in $(seq 1 20); do
  curl -fs -o /dev/null "$KC_URL/realms/partner" && break
  echo "Waiting for the provisioned realms at $KC_URL (run: docker compose run --rm terraform) ..."
  sleep 3
done
curl -fs -o /dev/null "$KC_URL/realms/partner" || { echo "Realms not found. Provision them first." >&2; exit 1; }

echo "Restarting both APIs, so payment-api starts with unpaid orders and both load the realm's keys ..."
docker compose up -d --force-recreate --wait inventory-api payment-api 2>/dev/null || {
  echo "Could not restart the APIs. Start the lab first: docker compose up -d --build --wait" >&2
  exit 1
}

ALICE_LOGIN=$(login alice "openid order-api")
ALICE_LOGIN_NO_AUD=$(login alice openid)
BOB_LOGIN=$(login bob "openid order-api")
INVENTORY_TOKEN=$(exchange "$ALICE_LOGIN" -d audience=inventory-api)
PAYMENT_TOKEN=$(exchange "$ALICE_LOGIN" -d audience=payment-api)
BOB_NO_ROLE_TOKEN=$(exchange "$BOB_LOGIN" -d audience=payment-api -d scope=payment-audience-only)
PARTNER_TOKEN=$(access_token "$(curl -sS "$PARTNER_TOKEN_URL" -u partner-app:partner-app-secret \
  -d grant_type=client_credentials)")

printf '\033[1m== Tokens ==\033[0m\n'
show_claims "Alice's token for inventory-api (exchanged by order-api, audience=inventory-api):" "$INVENTORY_TOKEN"
show_claims "Bob's token for payment-api (audience=payment-api, scope=payment-audience-only):" "$BOB_NO_ROLE_TOKEN"
show_claims "partner-app's token (client credentials in the partner realm):" "$PARTNER_TOKEN"

printf '\n\033[1m== Acceptance matrix ==\033[0m\n'
printf '%3s  %-46s %4s %4s\n' "#" "Request" "Want" "Got"
check 1 "No token, to inventory-api" 401 GET "$INVENTORY_URL/stock/SKU-1001" ""
check 2 "Alice's login token (aud order-api)" 401 GET "$INVENTORY_URL/stock/SKU-1001" "$ALICE_LOGIN"
check 3 "Alice's login token, no order-api scope" 401 GET "$INVENTORY_URL/stock/SKU-1001" "$ALICE_LOGIN_NO_AUD"
check 4 "Inventory token, to inventory-api" 200 GET "$INVENTORY_URL/stock/SKU-1001" "$INVENTORY_TOKEN"
check 5 "Inventory token, replayed at payment-api" 401 POST "$PAYMENT_URL/payments" "$INVENTORY_TOKEN" '{"order_id":"42"}'
check 6 "Bob: payment-api aud, no role, his order 43" 403 POST "$PAYMENT_URL/payments" "$BOB_NO_ROLE_TOKEN" '{"order_id":"43"}'
check 7 "Alice's payment token, her order 42" 201 POST "$PAYMENT_URL/payments" "$PAYMENT_TOKEN" '{"order_id":"42"}'
check 8 "Alice's payment token, order 42 again" 409 POST "$PAYMENT_URL/payments" "$PAYMENT_TOKEN" '{"order_id":"42"}'
check 9 "Alice's payment token, Bob's order 43" 403 POST "$PAYMENT_URL/payments" "$PAYMENT_TOKEN" '{"order_id":"43"}'
check 10 "partner realm token (aud payment-api)" 401 POST "$PAYMENT_URL/payments" "$PARTNER_TOKEN" '{"order_id":"42"}'

expires_at=$(payload "$INVENTORY_TOKEN" | jq .exp)
wait_seconds=$(( expires_at + LEEWAY_SECONDS + 1 - $(date +%s) ))
if (( wait_seconds > 0 )); then
  printf '     (waiting %ss until the inventory token is past exp + %ss leeway)\n' "$wait_seconds" "$LEEWAY_SECONDS"
  sleep "$wait_seconds"
fi
check 11 "Inventory token after it expired" 401 GET "$INVENTORY_URL/stock/SKU-1001" "$INVENTORY_TOKEN"

if (( FAILURES > 0 )); then
  printf '\n%s of 11 requests did not get the expected status.\n' "$FAILURES"
  exit 1
fi
printf '\nAll 11 requests got the expected status.\n'
