# Alice may read stock and create payments. Bob may only read stock.
locals {
  users = {
    alice = { first_name = "Alice", roles = ["stock_read", "payment_create"] }
    bob   = { first_name = "Bob", roles = ["stock_read"] }
  }
}

resource "keycloak_user" "demo" {
  for_each = local.users

  realm_id       = keycloak_realm.demo.id
  username       = each.key
  enabled        = true
  email          = "${each.key}@example.com"
  email_verified = true
  first_name     = each.value.first_name
  last_name      = "Example"

  initial_password {
    value     = var.user_passwords[each.key]
    temporary = false
  }
}

# exhaustive = false keeps the realm's default roles.
resource "keycloak_user_roles" "demo" {
  for_each = local.users

  realm_id   = keycloak_realm.demo.id
  user_id    = keycloak_user.demo[each.key].id
  exhaustive = false
  role_ids   = [for role in each.value.roles : keycloak_role.client[role].id]
}
