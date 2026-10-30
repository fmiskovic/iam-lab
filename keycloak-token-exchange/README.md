# Keycloak token exchange lab

Companion lab for **[Keycloak Token Exchange, Hands-On](https://iamfilip.dev/posts/keycloak-token-exchange/)**.

Runs Keycloak 26.8.1 in Docker, provisions a demo realm with Terraform, and walks through standard token exchange ([RFC 8693](https://datatracker.ietf.org/doc/html/rfc8693)): the happy path, downscoping with `audience`, and the two most common failures.

## Requirements

- Docker with Compose v2
- `curl` and `jq`

Terraform runs in a container, so you don't need it installed.

## Run it

```bash
git clone https://github.com/fmiskovic/iam-lab.git
cd iam-lab/keycloak-token-exchange

docker compose up -d --wait          # start Keycloak, wait until it is healthy
docker compose run --rm terraform    # provision the demo realm with Terraform
./exchange.sh                        # walk through every exchange from the post
```

Keycloak's admin console is at <http://localhost:8080> (`admin` / `admin`).

Clean up, including the Terraform state volume. `--profile tools` is needed because the state volume belongs to the `terraform` service, which plain `down` skips:

```bash
docker compose --profile tools down -v
```

## What gets provisioned

| Role in the exchange | Client | Type | What matters |
|---|---|---|---|
| Initial client | `shop-web` | public | May request the optional client scope `order-api`, whose audience mapper adds `order-api` to `aud` |
| Requester client | `order-api` | confidential | `standard_token_exchange_enabled = true`, full scope off |
| Target clients | `inventory-api`, `payment-api` | confidential, no flows | Own the client roles `stock:read`, `payment:create` |
| Off-limits target | `loyalty-api` | confidential, no flows | `order-api` has no role scope mapping for it |
| Unrelated app | `backoffice-web` | public | Its tokens don't include `order-api` |

User **alice** (password `alice`) has the client roles `stock:read`, `payment:create` and `points:read`.

| File | Contents |
|---|---|
| [`terraform/realm.tf`](terraform/realm.tf) | The `demo` realm |
| [`terraform/clients.tf`](terraform/clients.tf) | All clients, and the `order-api` client scope with its audience mapper |
| [`terraform/roles.tf`](terraform/roles.tf) | Client roles and `order-api`'s role scope mappings |
| [`terraform/users.tf`](terraform/users.tf) | Alice and her roles |

## Running Terraform yourself

With Terraform ≥ 1.11 installed and Keycloak running:

```bash
cd terraform
terraform init
terraform apply
```

State is then stored in `terraform/terraform.tfstate` (git-ignored).

## Lab shortcuts - don't copy these to production

- `shop-web` and `backoffice-web` allow the password grant (*Direct access grants*) so `curl` can log in. Real apps use the authorization code flow with PKCE.
- Keycloak runs in dev mode with `admin`/`admin`, and `order-api` has a fixed client secret.
- Terraform authenticates with the admin user's password. In real setups, use a dedicated service account client with only the admin roles it needs.
