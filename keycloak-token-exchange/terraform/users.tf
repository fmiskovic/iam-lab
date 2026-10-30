resource "keycloak_user" "alice" {
  realm_id       = keycloak_realm.demo.id
  username       = "alice"
  enabled        = true
  email          = "alice@example.com"
  email_verified = true
  first_name     = "Alice"
  last_name      = "Example"

  initial_password {
    value     = var.alice_password
    temporary = false
  }
}

# Alice may read stock, create payments and read loyalty points.
# exhaustive = false keeps the realm's default roles.
resource "keycloak_user_roles" "alice" {
  realm_id   = keycloak_realm.demo.id
  user_id    = keycloak_user.alice.id
  exhaustive = false

  role_ids = [
    keycloak_role.client["stock_read"].id,
    keycloak_role.client["payment_create"].id,
    keycloak_role.client["points_read"].id,
  ]
}
