variable "camunda_client_id" {
  default = "KGNwvEgmGEWskRON"
}

variable "camunda_client_secret" {
  default = "zrIsrYWp.HgYOg2eAgIuI~2_AtkmQqFr"
}

variable "camunda_api_url" {
  default = "https://api.cloud.camunda.io"
}

variable "camunda_audience" {
  default = "api.cloud.camunda.io"
}

variable "camunda_token_url" {
  default = "https://login.cloud.camunda.io/oauth/token"
}

terraform {
  required_providers {
    camunda = {
      source = "camunda-community-hub/camunda"
    }
  }
}

provider "camunda" {
  api_url       = var.camunda_api_url
  audience      = var.camunda_audience
  client_id     = var.camunda_client_id
  client_secret = var.camunda_client_secret
  token_url     = var.camunda_token_url
}
