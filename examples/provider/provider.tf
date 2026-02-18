# Get the client ID and client secret from https://console.cloud.camunda.io/
variable "camunda_client_id" {}
variable "camunda_client_secret" {}

provider "camunda" {
  client_id     = var.camunda_client_id
  client_secret = var.camunda_client_secret
}

# The channel containing the most recent version of Zeebe.
data "camunda_channel" "alpha" {
  name = "Alpha"
}

# A cluster plan type for default trials.
data "camunda_cluster_plan_type" "trial" {
  name = "Trial Cluster"
}

# The region associated with the trial plan.
data "camunda_region" "trial" {
  name = "Belgium, Europe (europe-west1)"
}

resource "camunda_cluster" "test" {
  name = "test"

  channel    = data.camunda_channel.alpha.id
  generation = data.camunda_channel.alpha.default_generation_id
  region     = data.camunda_region.trial.id
  plan_type  = data.camunda_cluster_plan_type.trial.id
}

output "cluster_id" {
  value = camunda_cluster.test.id
}
