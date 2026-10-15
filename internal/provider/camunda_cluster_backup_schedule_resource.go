package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type camundaClusterBackupScheduleData struct {
	Id        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	Schedule  types.String `tfsdk:"schedule"`
}

func NewCamundaClusterBackupScheduleResource() resource.Resource {
	set := func(op *op, plan camundaClusterBackupScheduleData) (camundaClusterBackupScheduleData, error) {
		plan.Id = plan.ClusterID
		return plan, op.client.SetBackupSchedule(op.ctx, plan.ClusterID.ValueString(), plan.Schedule.ValueString())
	}

	return &managedResource[camundaClusterBackupScheduleData]{
		typeName: "_cluster_backup_schedule",
		noun:     "cluster backup schedule",
		schema:   clusterBackupScheduleSchema,
		describe: func(d camundaClusterBackupScheduleData) string { return "cluster " + d.ClusterID.ValueString() },
		create: func(op *op, plan camundaClusterBackupScheduleData) (camundaClusterBackupScheduleData, error) {
			if err := op.client.WaitClusterHealthy(op.ctx, plan.ClusterID.ValueString()); err != nil {
				return plan, err
			}
			return set(op, plan)
		},
		read: func(op *op, prior camundaClusterBackupScheduleData) (camundaClusterBackupScheduleData, error) {
			// An import only sets the id, which is the cluster ID.
			prior.ClusterID = prior.Id
			schedule, err := op.client.GetBackupSchedule(op.ctx, prior.Id.ValueString())
			prior.Schedule = types.StringValue(schedule)
			return prior, err
		},
		update: func(op *op, prior, plan camundaClusterBackupScheduleData) (camundaClusterBackupScheduleData, error) {
			return set(op, plan)
		},
		delete: func(op *op, prior camundaClusterBackupScheduleData) error {
			return op.client.DeleteBackupSchedule(op.ctx, prior.ClusterID.ValueString())
		},
		importID: func(clusterID string) (camundaClusterBackupScheduleData, error) {
			return camundaClusterBackupScheduleData{Id: types.StringValue(clusterID)}, nil
		},
	}
}

func clusterBackupScheduleSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manage the backup schedule of a cluster on Camunda SaaS, so backups are taken automatically instead of on demand. " +
			"A cluster has at most one schedule. Destroying the resource stops scheduled backups; backups already taken are kept. " +
			"Only available if the cluster's plan supports backups.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID, which is the cluster ID",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "Cluster ID",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"schedule": schema.StringAttribute{
				MarkdownDescription: "Five-field cron expression in UTC (`minute hour day-of-month month day-of-week`), for example `0 2 * * *` for a daily backup at 02:00 UTC. Backups can be scheduled at most once every 15 minutes.",
				Required:            true,
			},
		},
	}
}
