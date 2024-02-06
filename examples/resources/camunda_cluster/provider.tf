variable "camunda_client_id" {
  description = "The client ID to connect to the Console API"
  type        = string
}

variable "camunda_client_secret" {
  description = "The client secret to connect to the Console API"
  type        = string
  sensitive   = true
}

variable "camunda_api_url" {
  description = "The Console API URL"
  default     = "https://api.cloud.camunda.io"
  type        = string
}

variable "camunda_audience" {
  description = "The audience to bind the authentication to"
  default     = "api.cloud.camunda.io"
  type        = string
}

variable "camunda_token_url" {
  description = "The authentication URL to fetch a token from"
  default     = "https://login.cloud.camunda.io/oauth/token"
  type        = string
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
