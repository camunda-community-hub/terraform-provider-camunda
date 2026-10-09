package provider

import (
	"context"
	"errors"
	"fmt"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &CamundaClusterResource{}
var _ resource.ResourceWithImportState = &CamundaClusterResource{}

type camundaClusterData struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Channel    types.String `tfsdk:"channel"`
	Region     types.String `tfsdk:"region"`
	PlanType   types.String `tfsdk:"plan_type"`
	Generation types.String `tfsdk:"generation"`
	AutoUpdate types.Bool   `tfsdk:"auto_update"`
}

type CamundaClusterResource struct {
	client *consoleClient
}

func NewCamundaClusterResource() resource.Resource {
	return &CamundaClusterResource{}
}

func (r *CamundaClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *CamundaClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a cluster on Camunda SaaS",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster ID",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster",
				Required:            true,
			},
			"channel": schema.StringAttribute{
				MarkdownDescription: "Channel",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"plan_type": schema.StringAttribute{
				MarkdownDescription: "Plan type",
				Required:            true,
			},
			"generation": schema.StringAttribute{
				MarkdownDescription: "Generation",
				Required:            true,
			},
			"auto_update": schema.BoolAttribute{
				MarkdownDescription: "Auto Update",
				Optional:            true,
				Default:             booldefault.StaticBool(true),
				Computed:            true,
			},
		},
	}
}

func (r *CamundaClusterResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	client, diags := consoleClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.client = client
}

func (r *CamundaClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data camundaClusterData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	clusterId, err := r.client.CreateCluster(ctx, console.CreateClusterRequest{
		Name:         data.Name.ValueString(),
		PlanTypeId:   data.PlanType.ValueString(),
		ChannelId:    data.Channel.ValueString(),
		GenerationId: data.Generation.ValueString(),
		RegionId:     data.Region.ValueString(),
		AutoUpdate:   data.AutoUpdate.ValueBoolPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cluster",
			fmt.Sprintf("Unable to create cluster, got error: %s", err),
		)
		return
	}

	data.Id = types.StringValue(clusterId)

	tflog.Info(ctx, "Camunda cluster created", map[string]interface{}{
		"clusterID": data.Id,
	})

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)

	// Creating a cluster takes some time, wait until it's marked healthy.
	if err := r.client.WaitClusterHealthy(ctx, clusterId); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cluster",
			fmt.Sprintf("Cluster %s never got healthy; got error: %s", clusterId, err),
		)
		return
	}
}

func (r *CamundaClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data camundaClusterData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	cluster, err := r.client.GetCluster(ctx, data.Id.ValueString())
	if errors.Is(err, errNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read cluster ID=%s, got error: %s", data.Id.ValueString(), err),
		)
		return
	}

	data.Name = types.StringValue(cluster.Name)
	data.Channel = types.StringValue(cluster.Channel.Uuid)
	data.Region = types.StringValue(cluster.Region.Uuid)
	data.PlanType = types.StringValue(cluster.PlanType.Uuid)
	data.Generation = types.StringValue(cluster.Generation.Uuid)
	data.AutoUpdate = types.BoolValue(cluster.AutoUpdate)

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state camundaClusterData

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// The management API only allows renaming a cluster in place. Refuse changes
	// to anything else instead of silently recording them in the state.
	if !plan.PlanType.Equal(state.PlanType) || !plan.Generation.Equal(state.Generation) || !plan.AutoUpdate.Equal(state.AutoUpdate) {
		resp.Diagnostics.AddError(
			"Unsupported cluster update",
			"Only the name of a cluster can be updated in place; changing plan_type, generation or auto_update is not supported.",
		)
		return
	}

	err := r.client.RenameCluster(ctx, state.Id.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to update cluster ID=%s, got error: %s", state.Id.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CamundaClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data camundaClusterData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteCluster(ctx, data.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to delete cluster ID=%s, got error: %s", data.Id.ValueString(), err),
		)
		return
	}
}

func (r *CamundaClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
