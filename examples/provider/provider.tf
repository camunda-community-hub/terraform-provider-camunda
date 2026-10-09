terraform {
  required_providers {
    camunda = {
      source  = "camunda-community-hub/camunda"
      version = "~> 0.1"
    }
  }
}

# Reads the credentials from CAMUNDA_CONSOLE_CLIENT_ID and
# CAMUNDA_CONSOLE_CLIENT_SECRET. See "Authentication" below.
provider "camunda" {}

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

  # Changing plan_type, generation or auto_update replaces the cluster and
  # deletes its data. prevent_destroy makes such a plan fail instead.
  lifecycle {
    prevent_destroy = true
  }
}

output "cluster_id" {
  value = camunda_cluster.test.id
}
