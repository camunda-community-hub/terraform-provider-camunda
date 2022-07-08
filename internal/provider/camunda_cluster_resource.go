package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	console "github.com/sijoma/console-customer-api-go"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ tfsdk.ResourceType = camundaClusterType{}
var _ tfsdk.Resource = camundaCluster{}
var _ tfsdk.ResourceWithImportState = camundaCluster{}

type camundaClusterType struct{}

func (t camundaClusterType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Manage a cluster on Camunda SaaS",

		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Computed:            true,
				MarkdownDescription: "Cluster ID",
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
				Type: types.StringType,
			},
			"name": {
				MarkdownDescription: "The name of the cluster",
				Required:            true,
				Type:                types.StringType,
			},
			"channel": {
				MarkdownDescription: "Channel",
				Required:            true,
				Type:                types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"region": {
				MarkdownDescription: "Region",
				Required:            true,
				Type:                types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"plan_type": {
				MarkdownDescription: "Plan type",
				Required:            true,
				Type:                types.StringType,
			},
			"generation": {
				MarkdownDescription: "Generation",
				Required:            true,
				Type:                types.StringType,
			},
		},
	}, nil
}

func (t camundaClusterType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return camundaCluster{
		provider: provider,
	}, diags
}

type camundaClusterData struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Channel    types.String `tfsdk:"channel"`
	Region     types.String `tfsdk:"region"`
	PlanType   types.String `tfsdk:"plan_type"`
	Generation types.String `tfsdk:"generation"`
}

type camundaCluster struct {
	provider provider
}

func (r camundaCluster) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	var data camundaClusterData

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	newClusterConfiguration := console.CreateClusterBody{
		Name:         data.Name.Value,
		PlanTypeId:   data.PlanType.Value,
		ChannelId:    data.Channel.Value,
		GenerationId: data.Generation.Value,
		RegionId:     data.Region.Value,
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

	data.Id = types.String{Value: inline.GetClusterId()}

	tflog.Info(ctx, "Camunda cluster created", map[string]interface{}{
		"clusterID": data.Id,
	})

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r camundaCluster) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	var data camundaClusterData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	cluster, _, err := r.provider.client.ClustersApi.GetCluster(ctx, data.Id.Value).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read cluster ID=%s, got error: %s", data.Id.Value, err.(*console.GenericOpenAPIError).Body()),
		)
		return
	}

	data.Name = types.String{Value: cluster.Name}
	data.Channel = types.String{Value: cluster.Channel.Uuid}
	data.Region = types.String{Value: cluster.Region.Uuid}
	data.PlanType = types.String{Value: cluster.PlanType.Uuid}
	data.Generation = types.String{Value: cluster.Generation.Uuid}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r camundaCluster) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
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

func (r camundaCluster) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var data camundaClusterData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	_, err := r.provider.client.ClustersApi.DeleteCluster(ctx, data.Id.Value).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to delete cluster ID=%s, got error: %s", data.Id.Value, err.(console.GenericOpenAPIError).Body()),
		)
		return
	}
}

func (r camundaCluster) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
