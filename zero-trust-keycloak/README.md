# Zero trust boundaries lab

Companion lab for **[Keycloak Is Not Your Zero Trust Architecture](https://iamfilip.dev/posts/zero-trust-keycloak/)**.

Runs Keycloak 26.8.1 in Docker, provisions two realms with Terraform, and starts two small Go APIs, `inventory-api` and `payment-api`, that validate Keycloak access tokens. [`matrix.sh`](matrix.sh) then sends eleven requests, each of which should succeed or fail for one specific reason, and checks that it does.

## Requirements

- Docker with Compose v2
- `curl` and `jq`
- Ports 8080, 8081 and 8082 free. If the [token exchange lab](../keycloak-token-exchange/) is running, stop it first (`docker compose stop` in its folder).

Terraform runs in a container and the APIs are built in one, so you need neither Terraform nor Go installed.

## Run it

```bash
git clone https://github.com/fmiskovic/iam-lab.git
cd iam-lab/zero-trust-keycloak

docker compose up -d --build --wait      # build the APIs, start Keycloak and both APIs
docker compose run --rm terraform        # provision the demo and partner realms
./matrix.sh                              # run the acceptance matrix (about 40 seconds)
```

`matrix.sh` restarts both APIs first, so `payment-api` starts with unpaid orders. It exits with status 1 if any request gets an unexpected status. Keycloak's admin console is at <http://localhost:8080> (`admin` / `admin`).

Clean up, including the Terraform state volume:

```bash
docker compose --profile tools down -v
```

## What the APIs check

Both APIs run the same code ([`api/`](api/)); `API_NAME` makes it `inventory-api` or `payment-api`. Every request to `GET /stock/{sku}` or `POST /payments` passes these checks, in this order:

| Check | Rejected with | Code |
|---|---|---|
| A bearer token is present | 401 | [`auth.go`](api/auth.go) |
| Signature with a key from the demo realm's JWKS, RS256 only | 401 | `auth.go` |
| `iss` is the configured issuer | 401 | `auth.go` |
| `exp` present and not passed, `nbf` and `iat` valid, 5 s leeway | 401 | `auth.go` |
| `aud` contains the API's own name; a missing `aud` fails | 401 | `auth.go` |
| `typ` is `Bearer`: Keycloak's access tokens, not ID tokens | 401 | `auth.go` |
| `azp` is an allowed client (`order-api`) | 401 | `auth.go` |
| The API's own client role: `stock:read` or `payment:create` | 403 | `auth.go` |
| `payment-api` only: the order exists, belongs to the caller and isn't paid yet; the amount comes from the order | 404 / 403 / 409 | [`orders.go`](api/orders.go), [`handlers.go`](api/handlers.go) |

The issuer and the JWKS URL are configured separately in [`docker-compose.yml`](docker-compose.yml): tokens are requested from your machine (`iss` is `http://localhost:8080/realms/demo`), while the APIs fetch keys inside the Docker network. Neither is ever taken from the token.

Each request is logged as one JSON line with a request ID, the status and the reason. The `Authorization` header is never logged.

Unit tests, if you have Go 1.27:

```bash
cd api && go test -race ./...
```

## What gets provisioned

| Realm | Client or user | What matters |
|---|---|---|
| `demo` | `shop-web` (public) | May request the optional scope `order-api`, which adds `order-api` to `aud` |
| `demo` | `order-api` (confidential) | Standard token exchange on, full scope off, role scope mappings for `stock:read` and `payment:create`; its tokens live **30 seconds** |
| `demo` | `inventory-api`, `payment-api` (no flows) | Own the client roles `stock:read` and `payment:create` |
| `demo` | `payment-audience-only` (optional scope of `order-api`) | Test fixture: hardcoded `payment-api` audience, no role |
| `demo` | alice / bob (password = username) | Alice has `stock:read` and `payment:create`; Bob only `stock:read` |
| `partner` | `partner-app` (client credentials) | Its tokens say `aud: payment-api`, signed with the partner realm's keys |

`payment-api` holds two orders in memory: 42 (Alice, EUR 129.90) and 43 (Bob, EUR 54.00).

## The matrix

| # | Request | Expected |
|---|---|---|
| 1 | No token, to `inventory-api` | 401 |
| 2 | Alice's login token (`aud: order-api`), to `inventory-api` | 401, wrong audience |
| 3 | Alice's login token without the `order-api` scope (no `aud`) | 401, audience required |
| 4 | Token exchanged for `inventory-api`, to `inventory-api` | 200 |
| 5 | The same token, replayed at `payment-api` | 401, wrong audience |
| 6 | Bob's token with the `payment-api` audience but no `payment:create`, for his own order | 403, missing role |
| 7 | Alice's `payment-api` token, her order 42 | 201 |
| 8 | The same token, order 42 again | 409, already paid |
| 9 | The same token, Bob's order 43 | 403, not her order |
| 10 | `partner-app` token with `aud: payment-api` from the partner realm | 401, key not trusted |
| 11 | The inventory token after `exp` + leeway | 401, expired |

## Files

| File | Contents |
|---|---|
| [`terraform/realm.tf`](terraform/realm.tf), [`terraform/partner.tf`](terraform/partner.tf) | The `demo` realm; the `partner` realm and its client |
| [`terraform/clients.tf`](terraform/clients.tf) | Clients, the `order-api` audience scope, the test fixture scope |
| [`terraform/roles.tf`](terraform/roles.tf) | Client roles and `order-api`'s role scope mappings |
| [`terraform/users.tf`](terraform/users.tf) | Alice, Bob and their roles |
| [`api/`](api/) | The Go APIs and their unit tests ([`golang-jwt`](https://github.com/golang-jwt/jwt) v5.3.1, [`keyfunc`](https://github.com/MicahParks/keyfunc) v3.8.2) |
| [`matrix.sh`](matrix.sh) | Gets the tokens and runs the eleven requests |

## Lab shortcuts - don't copy these to production

- `shop-web` allows the password grant (*Direct access grants*) so `curl` can log in. Real apps use the authorization code flow with PKCE.
- Keycloak runs in dev mode with `admin`/`admin`; `order-api` and `partner-app` have fixed client secrets. Terraform authenticates with the admin password.
- `payment-api` decides order ownership by `preferred_username` so the seed data stays readable. Use `sub`: it's stable and unique per issuer, usernames can change.
- The APIs' error descriptions name the failed check so you can see it. Production APIs usually tell the caller less and log the rest.
- Orders live in memory, and the `payment-audience-only` scope exists only to produce a token with an audience but no role.
