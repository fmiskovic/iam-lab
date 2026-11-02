# Initial client: the web app Alice and Bob log in to.
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

# Optional client scope that puts order-api into the aud claim, so order-api may exchange
# the token. Apps get it only when they ask for it (scope=order-api).
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

resource "keycloak_openid_client_optional_scopes" "shop_web" {
  realm_id        = keycloak_realm.demo.id
  client_id       = keycloak_openid_client.shop_web.id
  optional_scopes = [keycloak_openid_client_scope.order_api_audience.name]
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

  # Only the role scope mappings in roles.tf decide which audiences and roles
  # order-api can obtain. With full scope on, it could reach every API the user can.
  full_scope_allowed = false

  # Exchanged tokens are issued to order-api, so they live 30 seconds instead of the
  # realm's 5 minutes. Short enough that matrix.sh can wait for one to expire.
  access_token_lifespan = "30"
}

# Test fixture, not a pattern: an optional scope with a hardcoded payment-api audience.
# It lets order-api obtain a payment-api token for a user without payment:create, so
# matrix.sh can show that the role check stands on its own. Hardcoded audience mappers
# like this one add the audience whatever roles the user has.
resource "keycloak_openid_client_scope" "payment_audience_only" {
  realm_id               = keycloak_realm.demo.id
  name                   = "payment-audience-only"
  description            = "Test fixture: payment-api audience without any payment-api role"
  include_in_token_scope = true
}

resource "keycloak_openid_audience_protocol_mapper" "payment_audience_only" {
  realm_id                 = keycloak_realm.demo.id
  client_scope_id          = keycloak_openid_client_scope.payment_audience_only.id
  name                     = "audience: payment-api"
  included_client_audience = keycloak_openid_client.target["payment-api"].client_id
  add_to_access_token      = true
  add_to_id_token          = false
}

resource "keycloak_openid_client_optional_scopes" "order_api" {
  realm_id        = keycloak_realm.demo.id
  client_id       = keycloak_openid_client.order_api.id
  optional_scopes = [keycloak_openid_client_scope.payment_audience_only.name]
}

# Target clients: the two APIs. They only accept tokens, so no login flows are enabled.
locals {
  target_clients = {
    "inventory-api" = "Inventory API (target client)"
    "payment-api"   = "Payment API (target client)"
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
