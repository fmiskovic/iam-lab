# Initial client: the web app Alice logs in to.
resource "keycloak_openid_client" "shop_web" {
  realm_id  = keycloak_realm.demo.id
  client_id = "shop-web"
  name      = "Shop web app (initial client)"

  access_type                  = "PUBLIC"
  standard_flow_enabled        = true
  direct_access_grants_enabled = true # Lab only: lets curl log in with a password.
  valid_redirect_uris          = ["http://localhost:3000/*"]
  web_origins                  = ["+"]
  full_scope_allowed           = false
}

# Optional client scope that puts order-api into the aud claim. Without that audience,
# order-api is not allowed to exchange the token. As an optional scope, apps get it only
# when they ask for it (scope=order-api), not in every token they receive.
resource "keycloak_openid_client_scope" "order_api_audience" {
  realm_id               = keycloak_realm.demo.id
  name                   = "order-api"
  description            = "Adds order-api to the access token audience"
  include_in_token_scope = true
}

resource "keycloak_openid_audience_protocol_mapper" "order_api_audience" {
  realm_id                 = keycloak_realm.demo.id
  client_scope_id          = keycloak_openid_client_scope.order_api_audience.id
  name                     = "audience: order-api"
  included_client_audience = keycloak_openid_client.order_api.client_id
  add_to_access_token      = true
  add_to_id_token          = false
}

# shop-web may request the order-api scope. This resource is authoritative: it replaces
# Keycloak's default optional scopes (address, phone, offline_access, ...), which shop-web doesn't need.
resource "keycloak_openid_client_optional_scopes" "shop_web" {
  realm_id        = keycloak_realm.demo.id
  client_id       = keycloak_openid_client.shop_web.id
  optional_scopes = [keycloak_openid_client_scope.order_api_audience.name]
}

# Another app Alice uses. Its tokens are not meant for order-api.
resource "keycloak_openid_client" "backoffice_web" {
  realm_id  = keycloak_realm.demo.id
  client_id = "backoffice-web"
  name      = "Back-office web app (does not talk to order-api)"

  access_type                  = "PUBLIC"
  standard_flow_enabled        = true
  direct_access_grants_enabled = true # Lab only.
  valid_redirect_uris          = ["http://localhost:3001/*"]
  web_origins                  = ["+"]
  full_scope_allowed           = false
}

# Requester client: the service that performs the token exchange.
resource "keycloak_openid_client" "order_api" {
  realm_id  = keycloak_realm.demo.id
  client_id = "order-api"
  name      = "Order API (requester client)"

  access_type   = "CONFIDENTIAL"
  client_secret = var.order_api_secret

  standard_flow_enabled           = false
  direct_access_grants_enabled    = false
  service_accounts_enabled        = false
  standard_token_exchange_enabled = true

  # Only the role scope mappings in roles.tf decide which audiences
  # order-api can request. With full scope on, it could reach every API Alice can.
  full_scope_allowed = false
}

# Target clients: APIs that only accept tokens, so no login flows are enabled.
locals {
  target_clients = {
    "inventory-api" = "Inventory API (target client)"
    "payment-api"   = "Payment API (target client)"
    "loyalty-api"   = "Loyalty API (order-api is not allowed to call it)"
  }
}

resource "keycloak_openid_client" "target" {
  for_each = local.target_clients

  realm_id  = keycloak_realm.demo.id
  client_id = each.key
  name      = each.value

  access_type                  = "CONFIDENTIAL"
  standard_flow_enabled        = false
  direct_access_grants_enabled = false
  service_accounts_enabled     = false
}
