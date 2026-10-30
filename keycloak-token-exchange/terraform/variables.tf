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

variable "alice_password" {
  description = "Password of the demo user alice."
  type        = string
  sensitive   = true
  default     = "alice"
}
