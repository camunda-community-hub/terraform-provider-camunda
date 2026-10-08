package provider

import (
	"context"
	"fmt"
	"time"

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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
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

	Description types.String `tfsdk:"description"`
}

type CamundaClusterResource struct {
	provider *CamundaCloudProvider
}

func NewCamundaClusterResource() resource.Resource {
	return &CamundaClusterResource{}
}

func (r *CamundaClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *CamundaClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a cluster on Camunda SaaS.\n\n" +
			"Only `name` and `description` can be updated in place. Changing `plan_type`, `generation`, " +
			"`auto_update`, `channel` or `region` destroys and recreates the cluster, **deleting all data of the cluster**. " +
			"Use `lifecycle { prevent_destroy = true }` to guard against this.",

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
				MarkdownDescription: "Generation. Changing it replaces the cluster.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"auto_update": schema.BoolAttribute{
				MarkdownDescription: "Auto Update. Changing it replaces the cluster.",
				Optional:            true,
				Default:             booldefault.StaticBool(true),
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the cluster (max 150 characters)",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthAtMost(150)},
			},
		},
	}
}

func (r *CamundaClusterResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	provider, ok := req.ProviderData.(*CamundaCloudProvider)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *incidentio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.provider = provider
}

func (r *CamundaClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data camundaClusterData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	newClusterConfiguration := console.CreateClusterRequest{
		Name:         data.Name.ValueString(),
		PlanTypeId:   data.PlanType.ValueString(),
		ChannelId:    data.Channel.ValueString(),
		GenerationId: data.Generation.ValueString(),
		RegionId:     data.Region.ValueString(),
		AutoUpdate:   data.AutoUpdate.ValueBoolPointer(),
		Description:  data.Description.ValueStringPointer(),
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	inline, _, err := r.provider.client.DefaultAPI.CreateCluster(ctx).
		CreateClusterRequest(newClusterConfiguration).
		Execute()

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cluster",
			fmt.Sprintf("Unable to create cluster, got error: %s", formatClientError(err)),
		)
		return
	}

	clusterId := inline.GetClusterId()
	data.Id = types.StringValue(clusterId)

	tflog.Info(ctx, "Camunda cluster created", map[string]interface{}{
		"clusterID": data.Id,
	})

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)

	// Creating a cluster takes some time, wait until it's marked healthy.
	createState := &retry.StateChangeConf{
		// The cluster states that we need to keep waiting on
		Pending: []string{
			string(console.CLUSTERCOMPONENTSTATUS_CREATING),
			string(console.CLUSTERCOMPONENTSTATUS_UPDATING),
		},

		// The cluster states that we would like to reach
		Target: []string{
			string(console.CLUSTERCOMPONENTSTATUS_HEALTHY),
		},

		// How many times the target state has to be reached to continue.
		ContinuousTargetOccurence: 2,

		Refresh: func() (interface{}, string, error) {
			cluster, _, err := r.provider.client.DefaultAPI.
				GetCluster(ctx, clusterId).
				Execute()

			if err != nil {
				return nil, "", err
			}

			tflog.Info(ctx, "Camunda cluster status", map[string]interface{}{
				"clusterID":     cluster.Uuid,
				"clusterStatus": cluster.Status.Ready,
			})

			return cluster, string(cluster.Status.Ready), nil
		},

		Timeout:    30 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 5 * time.Second,
	}

	_, err = createState.WaitForStateContext(ctx)

	if err != nil {
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

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	cluster, response, err := r.provider.client.DefaultAPI.GetCluster(ctx, data.Id.ValueString()).Execute()
	if isNotFound(err, response) {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read cluster ID=%s, got error: %s", data.Id.ValueString(), formatClientError(err)),
		)
		return
	}

	data.Name = types.StringValue(cluster.Name)
	data.Channel = types.StringValue(cluster.Channel.Uuid)
	data.Region = types.StringValue(cluster.Region.Uuid)
	data.PlanType = types.StringValue(cluster.PlanType.Uuid)
	data.Generation = types.StringValue(cluster.Generation.Uuid)
	data.AutoUpdate = types.BoolValue(cluster.AutoUpdate)
	if cluster.Description == nil || *cluster.Description == "" {
		data.Description = types.StringNull()
	} else {
		data.Description = types.StringPointerValue(cluster.Description)
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

	// plan_type, generation and auto_update cannot be updated in place and force replacement,
	// so only name and description can differ here.
	if !plan.Name.Equal(state.Name) || !plan.Description.Equal(state.Description) {
		ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

		// An empty description clears it; omitting the field would leave it unchanged.
		description := plan.Description.ValueString()

		_, err := r.provider.client.DefaultAPI.UpdateCluster(ctx, state.Id.ValueString()).
			UpdateClusterBody(console.UpdateClusterBody{
				Name:        plan.Name.ValueStringPointer(),
				Description: &description,
			}).
			Execute()
		if err != nil {
			resp.Diagnostics.AddError(
				"Client Error",
				fmt.Sprintf("Unable to update cluster ID=%s, got error: %s", state.Id.ValueString(), formatClientError(err)),
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

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	_, err := r.provider.client.DefaultAPI.DeleteCluster(ctx, data.Id.ValueString()).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to delete cluster ID=%s, got error: %s", data.Id.ValueString(), formatClientError(err)),
		)
		return
	}
}

func (r *CamundaClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
