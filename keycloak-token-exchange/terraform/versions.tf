terraform {
  required_version = ">= 1.11"

  required_providers {
    keycloak = {
      source  = "keycloak/keycloak"
      version = "~> 5.10"
    }
  }

  # Local state. When you run Terraform yourself it lives next to this file;
  # the docker compose "terraform" service points it at a Docker volume instead.
  backend "local" {}
}

provider "keycloak" {
  url       = var.keycloak_url
  realm     = "master"
  client_id = "admin-cli"
  username  = var.admin_username
  password  = var.admin_password
}
