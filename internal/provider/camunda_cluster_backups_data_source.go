package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterBackupsDataSourceData struct {
	ClusterId types.String                  `tfsdk:"cluster_id"`
	Backups   []clusterBackupDataSourceItem `tfsdk:"backups"`
}

type clusterBackupDataSourceItem struct {
	Id             types.String `tfsdk:"id"`
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

func NewCamundaClusterBackupsDataSource() datasource.DataSource {
	return &managedDataSource[clusterBackupsDataSourceData]{
		typeName: "_cluster_backups",
		noun:     "cluster backups",
		schema:   clusterBackupsSchema,
		read: func(op *op, config clusterBackupsDataSourceData) (clusterBackupsDataSourceData, error) {
			listed, err := op.client.ListBackups(op.ctx, config.ClusterId.ValueString())
			if err != nil {
				return config, err
			}

			config.Backups = make([]clusterBackupDataSourceItem, 0, len(listed))
			for _, b := range listed {
				config.Backups = append(config.Backups, clusterBackupDataSourceItem{
					Id:             types.StringValue(b.ID),
					Name:           types.StringValue(b.Name),
					Created:        types.StringValue(b.Created),
					Completed:      nullIfEmpty(b.Completed),
					Status:         types.StringValue(b.Status),
					ZeebeStatus:    types.StringValue(b.ZeebeStatus),
					OperateStatus:  types.StringValue(b.OperateStatus),
					TasklistStatus: types.StringValue(b.TasklistStatus),
					OptimizeStatus: types.StringValue(b.OptimizeStatus),
					GenerationId:   nullIfEmpty(b.GenerationID),
					GenerationName: nullIfEmpty(b.GenerationName),
				})
			}
			return config, nil
		},
	}
}

func clusterBackupsSchema() schema.Schema {
	str := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: description}
	}
	status := "One of `In progress`, `Failed`, `Complete` or `-`."

	return schema.Schema{
		MarkdownDescription: "List the backups of a cluster, for example to pick one to restore.",

		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster ID",
			},
			"backups": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The backups of the cluster.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":              str("The ID of the backup."),
						"name":            str("The name of the backup."),
						"created":         str("When the backup was started."),
						"completed":       str("When the backup finished. Null while it is running."),
						"status":          str("Overall status of the backup. " + status),
						"zeebe_status":    str("Status of the Zeebe part of the backup. " + status),
						"operate_status":  str("Status of the Operate part of the backup. " + status),
						"tasklist_status": str("Status of the Tasklist part of the backup. " + status),
						"optimize_status": str("Status of the Optimize part of the backup. " + status),
						"generation_id":   str("ID of the generation the cluster ran when the backup was taken. Null for legacy backups."),
						"generation_name": str("Name of that generation. Null for legacy backups."),
					},
				},
			},
		},
	}
}
