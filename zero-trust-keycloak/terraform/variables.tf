variable "keycloak_url" {
  description = "Base URL of the Keycloak server Terraform talks to."
  type        = string
  default     = "http://localhost:8080"
}

variable "admin_username" {
  description = "Keycloak admin user (master realm)."
  type        = string
  default     = "admin"
}

variable "admin_password" {
  description = "Password of the Keycloak admin user."
  type        = string
  sensitive   = true
  default     = "admin"
}

variable "order_api_secret" {
  description = "Client secret of order-api. A fixed value keeps the lab's curl commands simple; never do this for real clients."
  type        = string
  sensitive   = true
  default     = "order-api-secret"
}

variable "partner_app_secret" {
  description = "Client secret of partner-app in the partner realm. Fixed for the same reason."
  type        = string
  sensitive   = true
  default     = "partner-app-secret"
}

variable "user_passwords" {
  description = "Passwords of the demo users. Each user's password is their username."
  type        = map(string)
  sensitive   = true
  default = {
    alice = "alice"
    bob   = "bob"
  }
}
