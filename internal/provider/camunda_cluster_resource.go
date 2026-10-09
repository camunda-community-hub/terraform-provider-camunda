package provider

import (
	"context"
	"errors"
	"fmt"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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

	Description       types.String `tfsdk:"description"`
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
		MarkdownDescription: "Manage a cluster on Camunda SaaS. " +
			"Only `name` and `description` can be updated in place. Changing `plan_type`, `generation` " +
			"(unless `auto_update` is enabled), `auto_update`, `channel` or `region` destroys and recreates the cluster, " +
			"**deleting all data of the cluster**. Use `lifecycle { prevent_destroy = true }` to guard against this.",

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
				MarkdownDescription: "Plan type. Changing it replaces the cluster.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"generation": schema.StringAttribute{
				MarkdownDescription: "Generation the cluster is created with. Changing it replaces the cluster, unless `auto_update` is enabled: Camunda then upgrades the cluster over time; see `current_generation` for the generation it actually runs.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						generationChangeReplaces,
						"Changing the generation replaces the cluster unless auto_update is enabled.",
						"Changing the generation replaces the cluster unless `auto_update` is enabled.",
					),
				},
			},
			"current_generation": schema.StringAttribute{
				MarkdownDescription: "Generation the cluster currently runs.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"auto_update": schema.BoolAttribute{
				MarkdownDescription: "Auto Update. Changing it replaces the cluster.",
				Optional:            true,
				Default:             booldefault.StaticBool(true),
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the cluster (1 to 150 characters). Remove the attribute to clear it.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 150)},
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
		Description:  data.Description.ValueStringPointer(),
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

	cluster, err := r.client.GetCluster(ctx, clusterId)
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read cluster ID=%s, got error: %s", clusterId, err),
		)
		return
	}
	data.setFromAPI(cluster)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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

	data.setFromAPI(cluster)

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

	// Every other change forces replacement, and a generation change while
	// auto_update is on needs no API call.
	if !plan.Name.Equal(state.Name) || !plan.Description.Equal(state.Description) {
		err := r.client.UpdateCluster(ctx, state.Id.ValueString(), plan.Name.ValueString(), plan.Description.ValueString())
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

// generationChangeReplaces lets the generation change in place while
// auto_update is on: Camunda then moves the cluster to newer generations
// itself, and the configured generation only records where it started.
func generationChangeReplaces(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	var stateAutoUpdate, planAutoUpdate types.Bool
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("auto_update"), &stateAutoUpdate)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("auto_update"), &planAutoUpdate)...)

	resp.RequiresReplace = !stateAutoUpdate.ValueBool() || !planAutoUpdate.ValueBool()
}

// setFromAPI copies what the Console reports about the cluster into the model.
func (d *camundaClusterData) setFromAPI(cluster *console.Cluster) {
	d.Name = types.StringValue(cluster.Name)
	d.Channel = types.StringValue(cluster.Channel.Uuid)
	d.Region = types.StringValue(cluster.Region.Uuid)
	d.PlanType = types.StringValue(cluster.PlanType.Uuid)
	d.AutoUpdate = types.BoolValue(cluster.AutoUpdate)
	if cluster.Description == nil || *cluster.Description == "" {
		d.Description = types.StringNull()
	} else {
		d.Description = types.StringPointerValue(cluster.Description)
	}
	d.CurrentGeneration = types.StringValue(cluster.Generation.Uuid)

	// With auto_update the server moves the cluster to newer generations, so
	// the configured generation is only where it started; keep it instead of
	// reporting a diff the user can't apply.
	if d.Generation.IsNull() || !cluster.AutoUpdate {
		d.Generation = d.CurrentGeneration
	}
}
