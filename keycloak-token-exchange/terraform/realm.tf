resource "keycloak_realm" "demo" {
  realm                 = "demo"
  enabled               = true
  access_token_lifespan = "5m"
}
