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
  debug         = false
}

#
#data "camunda_cloud_region" "eu_west" {
#  name = "Europe West"
#}
#
#data "camunda_cloud_regions" "all" {
#}
#
#data "camunda_cloud_regions" "europe" {
#  regex = ".*europe.*"
#}

locals {
  channels = {
    alpha  = "c767585c-eccc-4762-be78-3bfcd562ee1e"
    stable = "6bdf0d1c-3d5a-4df6-8d03-762682964d85"
  }

  generations = {
    "Zeebe 8.0.2"        = "edf8342a-ebeb-44f7-9280-356e9c36a1e2"
    "Zeebe 8.1.0-alpha1" = "c1f79896-8d0c-41d0-b8c5-0175157d32de"
  }
}

data "camunda_channel" "alpha" {
  name = "Alpha"
}

# data "camunda_generations" "alpha" {
#   channel = data.camunda_channel.alpha.id 

#   filter = "*8.1.0*"

#   # allowed = [{"name": "Zeebe 8.1.0-alpha1", "uuid": "c1f79896-8d0c-41d0-b8c5-0175157d32de"}]
# }

resource "camunda_cluster" "test" {
  name = "plop"

  channel    = data.camunda_channel.alpha.id
  generation = data.camunda_channel.alpha.default_generation_id

  region    = "2f6470f9-77ec-4be5-9cdc-3231caf683ec" // Europe West
  plan_type = "231932af-0223-4b60-9961-fe4f71800760" // Trial Package
}

output "cluster_id" {
  value = camunda_cluster.test.id
}

output "generation_name" {
  value = data.camunda_channel.alpha.default_generation_name
}
