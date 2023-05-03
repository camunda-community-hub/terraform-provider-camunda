package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	console "github.com/sijoma/console-customer-api-go"
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

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	newClusterConfiguration := console.CreateClusterBody{
		Name:         data.Name.ValueString(),
		PlanTypeId:   data.PlanType.ValueString(),
		ChannelId:    data.Channel.ValueString(),
		GenerationId: data.Generation.ValueString(),
		RegionId:     data.Region.ValueString(),
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	inline, _, err := r.provider.client.ClustersApi.CreateCluster(ctx).
		CreateClusterBody(newClusterConfiguration).
		Execute()

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cluster",
			fmt.Sprintf("Unable to create cluster, got error: %s", err.(*console.GenericOpenAPIError).Body()),
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
			string(console.CREATING),
			string(console.UPDATING),
		},

		// The cluster states that we would like to reach
		Target: []string{
			string(console.HEALTHY),
		},

		// How many times the target state has to be reached to continue.
		ContinuousTargetOccurence: 2,

		Refresh: func() (interface{}, string, error) {
			cluster, _, err := r.provider.client.ClustersApi.
				GetCluster(ctx, clusterId).
				Execute()

			if err != nil {
				return nil, "", err
			}

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

	cluster, response, err := r.provider.client.ClustersApi.GetCluster(ctx, data.Id.ValueString()).Execute()
	if err != nil && response.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read cluster ID=%s, got error: %s", data.Id.ValueString(), err.(*console.GenericOpenAPIError).Body()),
		)
		return
	}

	data.Name = types.StringValue(cluster.Name)
	data.Channel = types.StringValue(cluster.Channel.Uuid)
	data.Region = types.StringValue(cluster.Region.Uuid)
	data.PlanType = types.StringValue(cluster.PlanType.Uuid)
	data.Generation = types.StringValue(cluster.Generation.Uuid)

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data camundaClusterData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// example, err := d.provider.client.UpdateExample(...)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update example, got error: %s", err))
	//     return
	// }

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data camundaClusterData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	_, err := r.provider.client.ClustersApi.DeleteCluster(ctx, data.Id.ValueString()).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to delete cluster ID=%s, got error: %s", data.Id.ValueString(), err.(console.GenericOpenAPIError).Body()),
		)
		return
	}
}

func (r *CamundaClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
