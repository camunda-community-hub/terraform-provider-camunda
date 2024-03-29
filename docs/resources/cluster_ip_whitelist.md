---
page_title: "camunda_cluster_ip_whitelist Resource - terraform-provider-camunda"
subcategory: ""
description: |-
    Manage IP whitelists of a Camunda cluster
---

# camunda_cluster_ip_whitelist (Resource)

Manage IP whitelists of a Camunda cluster

This configure a cluster IP whitelist to authorize only the specified IP addresses to connect to the Camunda cluster.

~> **Note** Although you can create multiple instances of this resource for a
single cluster, they will overwrite each other in a random manner.
Instead, create a single `camunda_cluster_ip_whitelist` resource per-cluster, and configures
multiple `ip_whitelist` blocks inside this `camunda_cluster_ip_whitelist` resource.

## Example Usage

```terraform
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
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `cluster_id` (String) Cluster ID

### Optional

- `ip_whitelist` (Block Set) (see [below for nested schema](#nestedblock--ip_whitelist))

### Read-Only

- `id` (String) ID

<a id="nestedblock--ip_whitelist"></a>
### Nested Schema for `ip_whitelist`

Required:

- `ip` (String) The IP address/network to whitelist. Must be a valid IPv4 address/network (such as `10.0.0.1` or `172.42.0.0/24`)

Optional:

- `description` (String) A short description for this IP whitelist.
