variable "camunda_client_id" {}
variable "camunda_client_secret" {}
variable "camunda_api_url" {}
variable "camunda_audience" {}
variable "camunda_token_url" {}

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
