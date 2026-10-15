# Changelog

Release notes for every version, including breaking changes and upgrade steps, are on
[GitHub Releases](https://github.com/camunda-community-hub/terraform-provider-camunda/releases).

For upgrading from v0.0.x, see the [v0.1 upgrade guide](https://registry.terraform.io/providers/camunda-community-hub/camunda/latest/docs/guides/upgrading-to-0.1).

## Unreleased

Everything merged since v0.2.1. The version number is decided when the release is cut.

### New resources

* `camunda_cluster_backup`: take a backup of a cluster and wait until it is complete.
* `camunda_cluster_backup_schedule`: set the cron schedule backups are taken on.
* `camunda_cluster_secure_connectivity`: activate secure connectivity (AWS PrivateLink) for a cluster.
* `camunda_cluster_monitoring`: activate external monitoring (bring your own monitoring).
* `camunda_cluster_monitoring_client`: create credentials for scraping a cluster's metrics; `rotation_trigger` rotates the password.
* `camunda_cluster_encryption`: supply the customer-managed (BYOK) keys of a cluster created with `encryption = "External"`. The Console applies the keys once; changing them needs a new cluster.

### New data sources

* `camunda_cluster` and `camunda_clusters`
* `camunda_cluster_backups`
* `camunda_cluster_client` and `camunda_cluster_clients`
* `camunda_cluster_connector_secrets` (names only, never the values)
* `camunda_organization_member` and `camunda_organization_members`
* `camunda_ip_ranges`
* `camunda_audit_log`

### New actions

Terraform 1.14 or later; run them with `terraform apply -invoke=action.<type>.<name>`.

* `camunda_cluster_upgrade`: upgrade a cluster to the latest generation and wait until it is healthy. For a cluster with `auto_update = false`, set `generation` afterwards, or the next plan replaces the cluster.
* `camunda_cluster_wake`: resume a suspended cluster.
* `camunda_cluster_restore`: restore a backup onto its cluster.
* `camunda_cluster_failover`: fail over to the paired backup region. The source cluster can never be resumed.

### Changed

* `camunda_cluster` gains the optional `stage_label` and `hardware_packages` (changeable in place) and `backup_region` and `encryption` (changing them replaces the cluster), and the computed `owner_id`, `created`, `labels`, `status`, `links`, `encryption_status`, `encryption_primary_key_arn` and `encryption_secondary_key_arn`.
* Creating a cluster with `encryption = "External"` no longer waits for it to become healthy: it stops at `WaitingForInput` until its keys are supplied.
* A create or update that succeeds but fails to read the object back now saves what the write returned (IDs, one-time passwords) and taints the resource, instead of losing it.
