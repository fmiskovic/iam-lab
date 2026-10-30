# Client roles owned by the target APIs.
locals {
  client_roles = {
    stock_read     = { client = "inventory-api", name = "stock:read", description = "Read stock levels" }
    stock_write    = { client = "inventory-api", name = "stock:write", description = "Change stock levels" }
    payment_create = { client = "payment-api", name = "payment:create", description = "Create payments" }
    points_read    = { client = "loyalty-api", name = "points:read", description = "Read loyalty points" }
  }
}

resource "keycloak_role" "client" {
  for_each = local.client_roles

  realm_id    = keycloak_realm.demo.id
  client_id   = keycloak_openid_client.target[each.value.client].id
  name        = each.value.name
  description = each.value.description
}

# Role scope mappings of order-api: the only roles it may carry in exchanged tokens.
# The built-in "roles" client scope turns them into audiences (Audience Resolve mapper),
# so order-api can request inventory-api and payment-api, but never loyalty-api.
resource "keycloak_generic_role_mapper" "order_api" {
  for_each = toset(["stock_read", "payment_create"])

  realm_id  = keycloak_realm.demo.id
  client_id = keycloak_openid_client.order_api.id
  role_id   = keycloak_role.client[each.key].id
}
