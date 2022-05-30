
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
  client_id 	= var.camunda_client_id
  client_secret = var.camunda_client_secret
}

#data "camunda_cloud_channel" "alpha" {
#  name  = "Alpha"
#  regex = ""
#}
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

resource "camunda_cluster" "test" {
  name = "foobar"
  channel    =  "c767585c-eccc-4762-be78-3bfcd562ee1e" // Alpha channel
  region     = "2f6470f9-77ec-4be5-9cdc-3231caf683ec"  // Europe West
  plan_type  = "231932af-0223-4b60-9961-fe4f71800760" // Trial Package
  generation = "c1f79896-8d0c-41d0-b8c5-0175157d32de" // Zeebe 8.1.0-alpha1
}

output "cluster_id" {
	value = camunda_cluster.test.id
}