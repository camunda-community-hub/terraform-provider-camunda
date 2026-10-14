package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterFailoverActionData struct {
	ClusterID       types.String   `tfsdk:"cluster_id"`
	BackupIDs       []types.String `tfsdk:"backup_ids"`
	RestoreBackupID types.String   `tfsdk:"restore_backup_id"`
}

func NewCamundaClusterFailoverAction() action.Action {
	return &managedAction[clusterFailoverActionData]{
		typeName: "_cluster_failover",
		verb:     "fail over cluster",
		schema: func() schema.Schema {
			return schema.Schema{
				MarkdownDescription: "Trigger a regional failover: the Console creates a new cluster in the cluster's paired backup region from the given backups and restores one of them onto it. " +
					"**The source cluster is suspended and can never be resumed.** " +
					"Not enabled for all organizations. The new cluster is reported in the action's progress and can be read with the `camunda_clusters` data source; " +
					"it isn't managed by any resource, so import it into a `camunda_cluster` resource if you want Terraform to manage it. " +
					"Needs Terraform 1.14 or later.",
				Attributes: map[string]schema.Attribute{
					"cluster_id": clusterIDAttribute(),
					"backup_ids": schema.ListAttribute{
						MarkdownDescription: "IDs of the backups to clone into the failover cluster.",
						Required:            true,
						ElementType:         types.StringType,
					},
					"restore_backup_id": schema.StringAttribute{
						MarkdownDescription: "ID of the backup to restore onto the new cluster immediately. Must be one of `backup_ids`.",
						Required:            true,
					},
				},
			}
		},
		invoke: func(op *op, config clusterFailoverActionData, progress func(string)) error {
			clusterID := config.ClusterID.ValueString()
			backupIDs := make([]string, 0, len(config.BackupIDs))
			for _, id := range config.BackupIDs {
				backupIDs = append(backupIDs, id.ValueString())
			}

			result, err := op.client.TriggerFailover(op.ctx, clusterID, backupIDs, config.RestoreBackupID.ValueString())
			if err != nil {
				return fmt.Errorf("cluster %s: %w", clusterID, err)
			}
			progress(fmt.Sprintf("Failed over cluster %s to the new cluster %s: %s", clusterID, result.NewClusterID, result.Results))
			return nil
		},
	}
}
