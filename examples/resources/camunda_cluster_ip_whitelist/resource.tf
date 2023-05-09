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

resource "camunda_cluster_ip_whitelist" "test" {
  cluster_id = camunda_cluster.test.id

  # These IP whitelists are likely to prevent from connecting to your cluster :)
  ip_whitelist {
    ip          = "127.0.0.1"
    description = "localhost"
  }

  ip_whitelist {
    ip          = "192.168.0.0/24"
    description = "local network"
  }

  ip_whitelist {
    ip = "192.168.0.1"
    # no description
  }
}
