#!/usr/bin/env bash
# Walks through every step of "Keycloak Token Exchange, Hands-On".
# Requires: curl, jq, and the provisioned lab (see README.md).
set -euo pipefail

KC_URL="${KC_URL:-http://localhost:8080}"
TOKEN_URL="$KC_URL/realms/demo/protocol/openid-connect/token"
TOKEN_EXCHANGE="urn:ietf:params:oauth:grant-type:token-exchange"
ACCESS_TOKEN_TYPE="urn:ietf:params:oauth:token-type:access_token"

# Print the payload of a JWT (base64url-decoded, padding restored).
jwt_payload() {
  local payload
  payload=$(cut -d. -f2 <<<"$1" | tr '_-' '/+')
  case $(( ${#payload} % 4 )) in
    2) payload="$payload==" ;;
    3) payload="$payload=" ;;
  esac
  base64 -d <<<"$payload" | jq '{iss, sub, preferred_username, azp, aud, scope, resource_access, sid}'
}

# Log in as alice through a public client (direct access grant, lab only!).
# Args: client ID, requested scope.
login() {
  curl -fsS "$TOKEN_URL" \
    -d client_id="$1" \
    -d grant_type=password \
    -d username=alice \
    -d password=alice \
    --data-urlencode scope="$2" | jq -r .access_token
}

# order-api exchanges a subject token. Extra args are passed to curl.
exchange() {
  local subject_token=$1
  shift
  curl -sS "$TOKEN_URL" \
    -u order-api:order-api-secret \
    --data-urlencode grant_type="$TOKEN_EXCHANGE" \
    --data-urlencode subject_token_type="$ACCESS_TOKEN_TYPE" \
    -d subject_token="$subject_token" \
    "$@"
}

step() { printf '\n\033[1m== %s ==\033[0m\n' "$1"; }

until curl -fs -o /dev/null "$KC_URL/realms/demo" 2>/dev/null; do
  echo "Waiting for Keycloak at $KC_URL ..."
  sleep 3
done

step "1. Subject token issued to shop-web, with the order-api scope"
USER_TOKEN=$(login shop-web "openid order-api")
jwt_payload "$USER_TOKEN"

step "2. Exchange for a token that only inventory-api accepts"
EXCHANGED=$(exchange "$USER_TOKEN" -d audience=inventory-api)
jq 'del(.access_token)' <<<"$EXCHANGED"
jwt_payload "$(jq -r .access_token <<<"$EXCHANGED")"

step "3. Exchange without an audience parameter (no downscoping)"
jwt_payload "$(exchange "$USER_TOKEN" | jq -r .access_token)"

step "4. Ask for an audience order-api is not allowed to reach"
exchange "$USER_TOKEN" -d audience=loyalty-api | jq .

step "5. Exchange a token that was never meant for order-api"
OTHER_TOKEN=$(login backoffice-web openid)
jwt_payload "$OTHER_TOKEN"
exchange "$OTHER_TOKEN" -d audience=inventory-api | jq .
