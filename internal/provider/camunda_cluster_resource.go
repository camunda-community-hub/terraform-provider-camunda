package provider

import (
	"context"
	"errors"
	"fmt"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
var _ resource.ResourceWithModifyPlan = &CamundaClusterResource{}

type camundaClusterData struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Channel    types.String `tfsdk:"channel"`
	Region     types.String `tfsdk:"region"`
	PlanType   types.String `tfsdk:"plan_type"`
	Generation types.String `tfsdk:"generation"`
	AutoUpdate types.Bool   `tfsdk:"auto_update"`

	CurrentGeneration types.String `tfsdk:"current_generation"`
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
				MarkdownDescription: "Generation the cluster is created with. With `auto_update` enabled, Camunda upgrades the cluster over time; see `current_generation` for the generation it actually runs.",
				Required:            true,
			},
			"current_generation": schema.StringAttribute{
				MarkdownDescription: "Generation the cluster currently runs.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
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
	data.CurrentGeneration = data.Generation

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
	data.AutoUpdate = types.BoolValue(cluster.AutoUpdate)
	data.CurrentGeneration = types.StringValue(cluster.Generation.Uuid)

	// With auto_update the server moves the cluster to newer generations, so
	// the configured generation is only where it started; keep it instead of
	// reporting a diff the user can't apply.
	if data.Generation.IsNull() || !cluster.AutoUpdate {
		data.Generation = data.CurrentGeneration
	}

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

	// ModifyPlan rejects these already; this guards against values that were
	// still unknown at plan time.
	resp.Diagnostics.Append(immutableClusterChanges(plan, state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Name.Equal(state.Name) {
		err := r.client.RenameCluster(ctx, state.Id.ValueString(), plan.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Client Error",
				fmt.Sprintf("Unable to update cluster ID=%s, got error: %s", state.Id.ValueString(), err),
			)
			return
		}
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

// ModifyPlan rejects, at plan time, changes the management API cannot apply
// to an existing cluster.
func (r *CamundaClusterResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing to compare when creating or destroying.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var plan, state camundaClusterData
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(immutableClusterChanges(plan, state)...)
}

// immutableClusterChanges reports every planned change that would require
// recreating the cluster. Only the name can change in place; the generation
// may also change while auto_update is on, since the server owns it then.
func immutableClusterChanges(plan, state camundaClusterData) diag.Diagnostics {
	var diags diag.Diagnostics

	changed := func(planned, current attr.Value) bool {
		return !planned.IsUnknown() && !planned.Equal(current)
	}
	reject := func(attribute string) {
		diags.AddAttributeError(
			path.Root(attribute),
			fmt.Sprintf("Cannot change %s", attribute),
			fmt.Sprintf("%s can't be changed on an existing cluster. To change it, recreate the cluster with `terraform apply -replace=<cluster resource address>`.", attribute),
		)
	}

	if changed(plan.PlanType, state.PlanType) {
		reject("plan_type")
	}
	if changed(plan.AutoUpdate, state.AutoUpdate) {
		reject("auto_update")
	}
	if changed(plan.Generation, state.Generation) && !state.AutoUpdate.ValueBool() {
		reject("generation")
	}

	return diags
}
