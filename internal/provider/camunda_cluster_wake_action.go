package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
)

func NewCamundaClusterWakeAction() action.Action {
	return &managedAction[clusterActionData]{
		typeName: "_cluster_wake",
		verb:     "wake cluster",
		schema: func() schema.Schema {
			return schema.Schema{
				MarkdownDescription: "Resume a suspended cluster and wait until it is healthy. Needs Terraform 1.14 or later.",
				Attributes:          map[string]schema.Attribute{"cluster_id": clusterIDAttribute()},
			}
		},
		invoke: func(op *op, config clusterActionData, progress func(string)) error {
			clusterID := config.ClusterID.ValueString()

			if err := op.client.WakeCluster(op.ctx, clusterID); err != nil {
				return fmt.Errorf("cluster %s: %w", clusterID, err)
			}
			progress(fmt.Sprintf("Resuming cluster %s", clusterID))

			if err := op.client.WaitClusterResumed(op.ctx, clusterID); err != nil {
				return fmt.Errorf("cluster %s never became healthy after waking it: %w", clusterID, err)
			}
			progress(fmt.Sprintf("Cluster %s is healthy", clusterID))
			return nil
		},
	}
}
