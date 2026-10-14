package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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
						Validators:          []validator.List{listvalidator.SizeAtLeast(1)},
					},
					"restore_backup_id": schema.StringAttribute{
						MarkdownDescription: "ID of the backup to restore onto the new cluster immediately. Must be one of `backup_ids`.",
						Required:            true,
					},
				},
			}
		},
		validate: validateFailoverConfig,
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
			if len(result.Failures) > 0 {
				return fmt.Errorf("cluster %s was failed over to the new cluster %s, but the failover did not fully succeed: %s",
					clusterID, result.NewClusterID, strings.Join(result.Failures, "; "))
			}
			progress(fmt.Sprintf("Failed over cluster %s to the new cluster %s: %s", clusterID, result.NewClusterID, result.Results))
			return nil
		},
	}
}

// validateFailoverConfig checks that the backup to restore is one of the
// backups to clone, before the irreversible request is sent. It says nothing
// while either is not known yet.
func validateFailoverConfig(ctx context.Context, config tfsdk.Config) diag.Diagnostics {
	var diags diag.Diagnostics

	var restoreID types.String
	var backupIDs types.List
	diags.Append(config.GetAttribute(ctx, path.Root("restore_backup_id"), &restoreID)...)
	diags.Append(config.GetAttribute(ctx, path.Root("backup_ids"), &backupIDs)...)
	if diags.HasError() || restoreID.IsNull() || restoreID.IsUnknown() || backupIDs.IsNull() || backupIDs.IsUnknown() {
		return diags
	}

	for _, element := range backupIDs.Elements() {
		id, ok := element.(types.String)
		if !ok || id.IsUnknown() || id.Equal(restoreID) {
			return diags
		}
	}

	diags.AddAttributeError(
		path.Root("restore_backup_id"),
		"Backup to restore is not among the backups to clone",
		fmt.Sprintf("restore_backup_id %q must be one of backup_ids.", restoreID.ValueString()),
	)
	return diags
}
