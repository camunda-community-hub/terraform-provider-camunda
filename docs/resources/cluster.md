---
page_title: "camunda_cluster Resource - terraform-provider-camunda"
subcategory: ""
description: |-
    Manage a cluster on Camunda SaaS
---

# camunda_cluster (Resource)

Manage a cluster on Camunda SaaS

This creates a new Camunda cluster to which a new workflow can be deployed.

To connect a client to the cluster, use the `camunda_cluster_client` resource.

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
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `channel` (String) Channel
- `generation` (String) Generation
- `name` (String) The name of the cluster
- `plan_type` (String) Plan type
- `region` (String) Region

### Read-Only

- `id` (String) Cluster ID
