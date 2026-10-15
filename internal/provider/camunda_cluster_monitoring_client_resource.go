package provider

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type camundaClusterMonitoringClientData struct {
	Id              types.String `tfsdk:"id"`
	ClusterID       types.String `tfsdk:"cluster_id"`
	Username        types.String `tfsdk:"username"`
	RotationTrigger types.String `tfsdk:"rotation_trigger"`
	ClientID        types.String `tfsdk:"client_id"`
	Name            types.String `tfsdk:"name"`
	Created         types.String `tfsdk:"created"`
	CreatedBy       types.String `tfsdk:"created_by"`
	CreatedByName   types.String `tfsdk:"created_by_name"`
	Password        types.String `tfsdk:"password"`
}

func NewCamundaClusterMonitoringClientResource() resource.Resource {
	return &managedResource[camundaClusterMonitoringClientData]{
		typeName: "_cluster_monitoring_client",
		noun:     "cluster monitoring client",
		schema:   clusterMonitoringClientSchema,
		describe: func(d camundaClusterMonitoringClientData) string {
			return d.Username.ValueString() + " in cluster " + d.ClusterID.ValueString()
		},
		create: func(op *op, plan camundaClusterMonitoringClientData) (camundaClusterMonitoringClientData, error) {
			created, err := op.client.CreateMonitoringClient(op.ctx, plan.ClusterID.ValueString(), plan.Username.ValueString())
			if err != nil {
				return plan, err
			}
			plan.setFromAPI(created.monitoringClient)
			plan.Password = types.StringValue(created.Password)
			return plan, nil
		},
		read: func(op *op, prior camundaClusterMonitoringClientData) (camundaClusterMonitoringClientData, error) {
			found, err := op.client.GetMonitoringClient(op.ctx, prior.ClusterID.ValueString(), prior.ClientID.ValueString())
			if err != nil {
				return prior, err
			}
			prior.setFromAPI(*found)
			return prior, nil
		},
		// Only the rotation trigger can change in place: it rotates the password.
		update: func(op *op, prior, plan camundaClusterMonitoringClientData) (camundaClusterMonitoringClientData, error) {
			rotated, err := op.client.RotateMonitoringClientPassword(op.ctx, prior.ClusterID.ValueString(), prior.ClientID.ValueString())
			if err != nil {
				return plan, err
			}
			plan.Password = types.StringValue(rotated.Password)
			return plan, nil
		},
		delete: func(op *op, prior camundaClusterMonitoringClientData) error {
			return op.client.DeleteMonitoringClient(op.ctx, prior.ClusterID.ValueString(), prior.ClientID.ValueString())
		},
		importID: func(id string) (camundaClusterMonitoringClientData, error) {
			clusterID, clientID, err := splitImportID(id, "<cluster_id>/<client_id>")
			if err != nil {
				return camundaClusterMonitoringClientData{}, err
			}
			return camundaClusterMonitoringClientData{
				ClusterID: types.StringValue(clusterID),
				ClientID:  types.StringValue(clientID),
			}, nil
		},
	}
}

func (d *camundaClusterMonitoringClientData) setFromAPI(c monitoringClient) {
	d.Id = types.StringValue(d.ClusterID.ValueString() + "/" + c.ID)
	d.ClientID = types.StringValue(c.ID)
	d.Username = types.StringValue(c.Username)
	d.Name = types.StringValue(c.Name)
	d.Created = types.StringValue(c.Created.Format(time.RFC3339))
	d.CreatedBy = types.StringValue(c.CreatedBy)
	d.CreatedByName = types.StringValue(c.CreatedByName)
}

func clusterMonitoringClientSchema() schema.Schema {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: description,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		}
	}

	return schema.Schema{
		MarkdownDescription: "Manage a client that may scrape the metrics of a cluster with external monitoring, see `camunda_cluster_monitoring`. " +
			"The password is only returned when the client is created or its password is rotated, so it can't be imported. " +
			"To rotate it, change `rotation_trigger`.",

		Attributes: map[string]schema.Attribute{
			"id": computed("Monitoring client ID, in the form `<cluster_id>/<client_id>`"),
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "Cluster ID",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "The username to scrape with. Changing it replaces the client.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"rotation_trigger": schema.StringAttribute{
				MarkdownDescription: "Any value; changing it rotates the client's password and stores the new one in `password`. For example, set it to a `time_rotating` resource's `rotation_rfc3339`.",
				Optional:            true,
			},
			"client_id":       computed("The ID of the client."),
			"name":            computed("The name of the client."),
			"created":         computed("When the client was created, as an RFC 3339 timestamp."),
			"created_by":      computed("ID of the user that created the client."),
			"created_by_name": computed("Name of the user that created the client."),
			"password": schema.StringAttribute{
				MarkdownDescription: "The client's password. Only known after creating the client or rotating its password.",
				Computed:            true,
				Sensitive:           true,
			},
		},
	}
}
