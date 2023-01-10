# The channel containing the most recent version of Zeebe.
data "camunda_channel" "alpha" {
  name = "Alpha"
}

# A cluster plan type for default trials.
data "camunda_cluster_plan_type" "trial" {
  name = "Trial Package"
}

# The region associated with the trial plan.
data "camunda_region" "trial" {
  name = data.camunda_cluster_plan_type.trial.region_name
}

resource "camunda_cluster" "test" {
  name = "test"

  channel    = data.camunda_channel.alpha.id
  generation = data.camunda_channel.alpha.default_generation_id
  region     = data.camunda_region.trial.id
  plan_type  = data.camunda_cluster_plan_type.trial.id
}
