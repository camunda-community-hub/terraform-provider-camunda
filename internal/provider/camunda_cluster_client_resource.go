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
var _ tfsdk.ResourceType = camundaClusterClientType{}
var _ tfsdk.Resource = camundaClusterClient{}
var _ tfsdk.ResourceWithImportState = camundaClusterClient{}

type camundaClusterClientType struct{}

func (t camundaClusterClientType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Manage a cluster client on Camunda SaaS",

		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Computed:            true,
				MarkdownDescription: "Cluster Client ID",
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
				Type: types.StringType,
			},
			"cluster_id": {
				MarkdownDescription: "Cluster ID",
				Required:            true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
				Type: types.StringType,
			},
			"name": {
				MarkdownDescription: "The name of the cluster client",
				Required:            true,
				Type:                types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"secret": {
				Computed:            true,
				MarkdownDescription: "The client secret",
				Type:                types.StringType,
				Sensitive:           true,
			},
			"zeebe_address": {
				Computed:            true,
				MarkdownDescription: "Zeebe Address",
				Type:                types.StringType,
			},
			"zeebe_client_id": {
				Computed:            true,
				MarkdownDescription: "Zeebe Client Id",
				Type:                types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"zeebe_authorization_server_url": {
				Computed:            true,
				MarkdownDescription: "Zeebe Authorization Server Url",
				Type:                types.StringType,
			},
		},
	}, nil
}

func (t camundaClusterClientType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return camundaClusterClient{
		provider: provider,
	}, diags
}

type camundaClusterClientData struct {
	Id        types.String `tfsdk:"id"`
	ClusterId types.String `tfsdk:"cluster_id"`
	Name      types.String `tfsdk:"name"`
	Secret    types.String `tfsdk:"secret"`

	ZeebeAddress                types.String `tfsdk:"zeebe_address"`
	ZeebeClientId               types.String `tfsdk:"zeebe_client_id"`
	ZeebeAuthorizationServerUrl types.String `tfsdk:"zeebe_authorization_server_url"`
}

type camundaClusterClient struct {
	provider provider
}

func (r camundaClusterClient) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	var data camundaClusterClientData

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	newClusterClientConfiguration := console.CreateClusterClientBody{
		ClientName: data.Name.Value,
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	inline, _, err := r.provider.client.ClientsApi.
		CreateClient(ctx, data.ClusterId.Value).
		CreateClusterClientBody(newClusterClientConfiguration).
		Execute()

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cluster",
			fmt.Sprintf("Unable to create cluster, got error: %s", err.(*console.GenericOpenAPIError).Body()),
		)
		return
	}

	data.Id = types.String{Value: inline.Uuid}
	data.ZeebeClientId = types.String{Value: inline.ClientId}
	data.Secret = types.String{Value: inline.ClientSecret}

	tflog.Info(ctx, "Camunda cluster client created", map[string]interface{}{
		"Id": data.Id,
	})

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r camundaClusterClient) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	var data camundaClusterClientData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	client, _, err := r.provider.client.ClientsApi.GetClient(ctx, data.ClusterId.Value, data.ZeebeClientId.Value).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read cluster client ID=%s, got error: %s", data.Id.Value, err.(*console.GenericOpenAPIError).Body()),
		)
		return
	}

	data.Name = types.String{Value: client.Name}
	data.ZeebeClientId = types.String{Value: client.ZEEBE_CLIENT_ID}
	data.ZeebeAddress = types.String{Value: client.ZEEBE_ADDRESS}
	data.ZeebeAuthorizationServerUrl = types.String{Value: client.ZEEBE_AUTHORIZATION_SERVER_URL}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r camundaClusterClient) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	var data camundaClusterClientData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r camundaClusterClient) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var data camundaClusterClientData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	_, err := r.provider.client.ClientsApi.DeleteClient(ctx, data.ClusterId.Value, data.ZeebeClientId.Value).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to delete cluster client ID=%s, got error: %s", data.Id.Value, err.(console.GenericOpenAPIError).Body()),
		)
		return
	}
}

func (r camundaClusterClient) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
