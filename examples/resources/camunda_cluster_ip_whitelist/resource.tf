variable "camunda_cluster_plan_type" {
  description = "The Camunda SaaS cluster plan type to use"
  default     = "Trial"
  type        = string
}

variable "camunda_region" {
  description = "The Camunda SaaS region in which to create the cluster"
  default     = "Belgium, Europe (europe-west1)"
  type        = string
}

data "camunda_channel" "this" {
  name = "Stable"
}

data "camunda_cluster_plan_type" "this" {
  name = var.camunda_cluster_plan_type
}

data "camunda_region" "this" {
  name = var.camunda_region
}

resource "camunda_cluster" "test" {
  name = "test"

  channel    = data.camunda_channel.this.id
  generation = data.camunda_channel.this.default_generation_id
  region     = data.camunda_region.this.id
  plan_type  = data.camunda_cluster_plan_type.this.id
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
