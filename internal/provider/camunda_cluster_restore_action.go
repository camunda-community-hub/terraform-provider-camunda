package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterRestoreActionData struct {
	ClusterID types.String `tfsdk:"cluster_id"`
	BackupID  types.String `tfsdk:"backup_id"`
}

func NewCamundaClusterRestoreAction() action.Action {
	return &managedAction[clusterRestoreActionData]{
		typeName: "_cluster_restore",
		verb:     "restore cluster",
		schema: func() schema.Schema {
			return schema.Schema{
				MarkdownDescription: "Restore a backup onto its cluster and wait until the restore has completed. " +
					"**The cluster's current data is replaced by the backup**, and the cluster is suspended until the restore is in flight. " +
					"Fails if the backup can't be restored, for example while another restore is running. Needs Terraform 1.14 or later.",
				Attributes: map[string]schema.Attribute{
					"cluster_id": clusterIDAttribute(),
					"backup_id": schema.StringAttribute{
						MarkdownDescription: "ID of the backup to restore, see `camunda_cluster_backup` and `camunda_cluster_backups`.",
						Required:            true,
					},
				},
			}
		},
		invoke: func(op *op, config clusterRestoreActionData, progress func(string)) error {
			clusterID, backupID := config.ClusterID.ValueString(), config.BackupID.ValueString()

			if _, err := op.client.RestoreBackup(op.ctx, clusterID, backupID); err != nil {
				return fmt.Errorf("backup %s in cluster %s: %w", backupID, clusterID, err)
			}
			progress(fmt.Sprintf("Restoring backup %s onto cluster %s", backupID, clusterID))

			if err := op.client.WaitRestoreComplete(op.ctx, clusterID, backupID); err != nil {
				return fmt.Errorf("restoring backup %s in cluster %s never completed: %w", backupID, clusterID, err)
			}
			progress(fmt.Sprintf("Restored backup %s onto cluster %s", backupID, clusterID))
			return nil
		},
	}
}
