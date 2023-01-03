# Get the client ID and client secret from https://console.cloud.camunda.io/
variable "camunda_client_id" {}
variable "camunda_client_secret" {}

provider "camunda" {
  client_id     = var.camunda_client_id
  client_secret = var.camunda_client_secret
}
