package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type camundaClusterBackupData struct {
	Id             types.String `tfsdk:"id"`
	ClusterId      types.String `tfsdk:"cluster_id"`
	BackupId       types.String `tfsdk:"backup_id"`
	Name           types.String `tfsdk:"name"`
	Created        types.String `tfsdk:"created"`
	Completed      types.String `tfsdk:"completed"`
	Status         types.String `tfsdk:"status"`
	ZeebeStatus    types.String `tfsdk:"zeebe_status"`
	OperateStatus  types.String `tfsdk:"operate_status"`
	TasklistStatus types.String `tfsdk:"tasklist_status"`
	OptimizeStatus types.String `tfsdk:"optimize_status"`
	GenerationId   types.String `tfsdk:"generation_id"`
	GenerationName types.String `tfsdk:"generation_name"`
}

func NewCamundaClusterBackupResource() resource.Resource {
	return &managedResource[camundaClusterBackupData]{
		typeName: "_cluster_backup",
		noun:     "cluster backup",
		schema:   clusterBackupSchema,
		describe: func(d camundaClusterBackupData) string {
			return d.BackupId.ValueString() + " in cluster " + d.ClusterId.ValueString()
		},
		create: func(op *op, plan camundaClusterBackupData) (camundaClusterBackupData, error) {
			clusterID := plan.ClusterId.ValueString()
			// A cluster that is still starting or updating can't take a backup.
			if err := op.client.WaitClusterHealthy(op.ctx, clusterID); err != nil {
				return plan, err
			}

			created, err := op.client.CreateBackup(op.ctx, clusterID)
			if err != nil {
				return plan, err
			}
			plan.setFromAPI(*created)
			return plan, nil
		},
		// Taking a backup takes some time, wait until it's complete.
		awaitReady: func(op *op, created camundaClusterBackupData) error {
			return op.client.WaitBackupComplete(op.ctx, created.ClusterId.ValueString(), created.BackupId.ValueString())
		},
		read: func(op *op, prior camundaClusterBackupData) (camundaClusterBackupData, error) {
			found, err := op.client.GetBackup(op.ctx, prior.ClusterId.ValueString(), prior.BackupId.ValueString())
			if err != nil {
				return prior, err
			}
			prior.setFromAPI(*found)
			return prior, nil
		},
		delete: func(op *op, prior camundaClusterBackupData) error {
			return op.client.DeleteBackup(op.ctx, prior.ClusterId.ValueString(), prior.BackupId.ValueString())
		},
		importID: func(id string) (camundaClusterBackupData, error) {
			clusterID, backupID, err := splitImportID(id, "<cluster_id>/<backup_id>")
			if err != nil {
				return camundaClusterBackupData{}, err
			}
			return camundaClusterBackupData{
				ClusterId: types.StringValue(clusterID),
				BackupId:  types.StringValue(backupID),
			}, nil
		},
	}
}

func (d *camundaClusterBackupData) setFromAPI(b backup) {
	d.Id = types.StringValue(d.ClusterId.ValueString() + "/" + b.ID)
	d.BackupId = types.StringValue(b.ID)
	d.Name = types.StringValue(b.Name)
	d.Created = types.StringValue(b.Created)
	d.Completed = nullIfEmpty(b.Completed)
	d.Status = types.StringValue(b.Status)
	d.ZeebeStatus = types.StringValue(b.ZeebeStatus)
	d.OperateStatus = types.StringValue(b.OperateStatus)
	d.TasklistStatus = types.StringValue(b.TasklistStatus)
	d.OptimizeStatus = types.StringValue(b.OptimizeStatus)
	d.GenerationId = nullIfEmpty(b.GenerationID)
	d.GenerationName = nullIfEmpty(b.GenerationName)
}

func clusterBackupSchema() schema.Schema {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: description}
	}
	// Everything but the status is fixed once the backup exists.
	fixed := func(description string) schema.StringAttribute {
		attr := computed(description)
		attr.PlanModifiers = []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
		return attr
	}
	status := "One of `In progress`, `Failed`, `Complete` or `-`."

	return schema.Schema{
		MarkdownDescription: "Take a backup of a cluster on Camunda SaaS. " +
			"Creating the resource starts a backup and waits until it is complete; a failed backup fails the creation. " +
			"Backups can't be changed, so a change to `cluster_id` creates a new backup, and destroying the resource deletes the backup. " +
			"Only available if the cluster's plan supports backups.",

		Attributes: map[string]schema.Attribute{
			"id": fixed("Backup ID, in the form `<cluster_id>/<backup_id>`"),
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "Cluster ID",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"backup_id":       fixed("The ID of the backup, to use when restoring it."),
			"name":            fixed("The name of the backup."),
			"created":         fixed("When the backup was started."),
			"completed":       computed("When the backup finished."),
			"status":          computed("Overall status of the backup. " + status),
			"zeebe_status":    computed("Status of the Zeebe part of the backup. " + status),
			"operate_status":  computed("Status of the Operate part of the backup. " + status),
			"tasklist_status": computed("Status of the Tasklist part of the backup. " + status),
			"optimize_status": computed("Status of the Optimize part of the backup. " + status),
			"generation_id":   fixed("ID of the generation the cluster ran when the backup was taken. Null for legacy backups."),
			"generation_name": fixed("Name of that generation. Null for legacy backups."),
		},
	}
}
