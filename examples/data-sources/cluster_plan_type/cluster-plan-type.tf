variable "camunda_client_id" {}
variable "camunda_client_secret" {}

terraform {
  required_providers {
    camunda = {
      source = "multani/camunda"
    }
  }
}


provider "camunda" {
  client_id     = var.camunda_client_id
  client_secret = var.camunda_client_secret
}

data "camunda_cluster_plan_type" "trial_package" {
  name = "Trial Package"
}

output "data" {
  value = data.camunda_cluster_plan_type.trial_package
}
