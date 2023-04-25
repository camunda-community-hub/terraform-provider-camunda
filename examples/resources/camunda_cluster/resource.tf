# The channel containing the most recent version of Zeebe.
data "camunda_channel" "alpha" {
  name = "Alpha"
}

# A cluster plan type for default trials.
data "camunda_cluster_plan_type" "trial" {
  name = "Trial Cluster"
}

# An available region
data "camunda_region" "europe" {
  name = "Belgium, Europe (europe-west1)"
}

resource "camunda_cluster" "test" {
  name = "test"

  channel    = data.camunda_channel.alpha.id
  generation = data.camunda_channel.alpha.default_generation_id
  region     = data.camunda_region.europe.id
  plan_type  = data.camunda_cluster_plan_type.trial.id
}
