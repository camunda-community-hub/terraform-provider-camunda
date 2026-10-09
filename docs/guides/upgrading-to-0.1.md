---
page_title: "Upgrading to v0.1"
subcategory: "Upgrade Guides"
description: |-
  What changes when you upgrade the Camunda provider from v0.0.x to v0.1, and how to upgrade without replacing clusters.
---

# Upgrading to v0.1

Before v0.1, some changes were saved to Terraform state but never applied in Camunda. v0.1 applies the
changes the Administration API supports and replaces the resource for the ones it doesn't. It also
reads more attributes back from Camunda. As a result, **the first plan after upgrading can show
changes you didn't make in this run**, including cluster replacements.

## Upgrade steps

1. Add `lifecycle { prevent_destroy = true }` to every cluster you can't lose:

   ```terraform
   resource "camunda_cluster" "prod" {
     # ...

     lifecycle {
       prevent_destroy = true
     }
   }
   ```

1. Update the version constraint and the lock file:

   ```terraform
   terraform {
     required_providers {
       camunda = {
         source  = "camunda-community-hub/camunda"
         version = "~> 0.1"
       }
     }
   }
   ```

   ```shell
   terraform init -upgrade
   ```

1. Run `terraform plan` and read it before you apply. Use the next section to decide what to do with each change.

## Reading the first plan

| Plan shows | Why | What to do |
|---|---|---|
| `camunda_cluster` must be replaced because of `plan_type`, `region`, `channel` or `auto_update` | Your configuration holds a value that an earlier version saved to state but never applied. Camunda still runs the old value. | To keep the cluster, set the attribute back to the value Camunda runs. The plan shows it. To move to the new value, remove `prevent_destroy` and accept the replacement, **which deletes the cluster and its data**. |
| `camunda_cluster` must be replaced because of `generation` | `auto_update` is off and Camunda runs a different generation than the configured one. | Set `generation` to the cluster's `current_generation`, or turn on `auto_update`. |
| `camunda_cluster` updated in place: `name` | An earlier version saved a rename to state without renaming the cluster. | Apply. v0.1 renames the cluster. |
| `camunda_cluster_client`, `camunda_cluster_connector_secret` or `camunda_cluster_ip_whitelist` must be replaced | The configured `cluster_id` or `name` differs from the one in Camunda. Earlier versions recorded such changes without moving the object. | Apply if you want the object in its configured place. A replaced secret is recreated under its new name, and the old secret is deleted. |
| `camunda_cluster_client` must be replaced because of `scopes` | The client's scopes were changed outside Terraform. | Apply to restore the configured scopes, or update your configuration to match. |

A plan that is empty, or that shows only changes you expect, is safe to apply.

## Other changes

* `generation` on a cluster with `auto_update = true` keeps the configured value when Camunda upgrades
  the cluster. The new read-only `current_generation` attribute shows what the cluster runs.
* Every resource can now be imported. See each resource's Import section.
* Destroying `camunda_cluster_ip_whitelist` now clears the allowlist. Earlier versions left it in place.
* The provider refreshes its OAuth token during long runs, so applies that create clusters no longer fail when the token expires.
