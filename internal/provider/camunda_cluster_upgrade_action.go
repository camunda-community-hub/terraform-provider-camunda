package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterActionData struct {
	ClusterID types.String `tfsdk:"cluster_id"`
}

func clusterIDAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "Cluster ID",
		Required:            true,
	}
}

func NewCamundaClusterUpgradeAction() action.Action {
	return &managedAction[clusterActionData]{
		typeName: "_cluster_upgrade",
		verb:     "upgrade cluster",
		schema: func() schema.Schema {
			return schema.Schema{
				MarkdownDescription: "Upgrade a cluster to the latest generation available and wait until it is healthy again. " +
					"Fails if there is no upgrade, if the upgrade is blocked, or if the cluster isn't ready for it. " +
					"The upgrade can't be undone. Needs Terraform 1.14 or later.\n\n" +
					"**A cluster managed by `camunda_cluster` with `auto_update = false`:** after the upgrade, set its `generation` to the generation it now runs " +
					"(for example the `default_generation_id` of the `camunda_channel` data source). Otherwise the next apply sees a different generation than configured " +
					"and plans to replace the cluster, **deleting all its data**. Guard the cluster with `lifecycle { prevent_destroy = true }` and check the plan before applying. " +
					"With `auto_update = true` there is nothing to reconcile.",
				Attributes: map[string]schema.Attribute{"cluster_id": clusterIDAttribute()},
			}
		},
		invoke: func(op *op, config clusterActionData, progress func(string)) error {
			clusterID := config.ClusterID.ValueString()

			upgrade, err := op.client.UpgradeCluster(op.ctx, clusterID)
			if err != nil {
				return fmt.Errorf("cluster %s: %w", clusterID, err)
			}
			progress(fmt.Sprintf("Upgrading cluster %s from generation %s to %s", clusterID, upgrade.From, upgrade.To))

			if err := op.client.WaitClusterUpgraded(op.ctx, clusterID, upgrade.ToID); err != nil {
				return fmt.Errorf("cluster %s never became healthy after the upgrade: %w", clusterID, err)
			}
			progress(fmt.Sprintf("Cluster %s is healthy on generation %s", clusterID, upgrade.To))
			return nil
		},
	}
}
