# A second realm on the same Keycloak server, with its own signing keys. Its partner-app
# can get tokens whose aud says payment-api. Signed correctly, by the same server, but not
# by the realm payment-api trusts.
resource "keycloak_realm" "partner" {
  realm   = "partner"
  enabled = true
}

resource "keycloak_openid_client" "partner_app" {
  realm_id  = keycloak_realm.partner.id
  client_id = "partner-app"
  name      = "Partner app (another realm)"

  access_type                  = "CONFIDENTIAL"
  client_secret                = var.partner_app_secret
  standard_flow_enabled        = false
  direct_access_grants_enabled = false
  service_accounts_enabled     = true # client credentials grant
}

resource "keycloak_openid_audience_protocol_mapper" "partner_app_payment_audience" {
  realm_id                 = keycloak_realm.partner.id
  client_id                = keycloak_openid_client.partner_app.id
  name                     = "audience: payment-api"
  included_custom_audience = "payment-api"
  add_to_access_token      = true
  add_to_id_token          = false
}
