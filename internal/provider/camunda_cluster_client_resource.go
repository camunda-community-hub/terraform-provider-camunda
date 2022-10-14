package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	console "github.com/sijoma/console-customer-api-go"

	"github.com/multani/terraform-provider-camunda/internal/validators"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &CamundaClusterClientResource{}
var _ resource.ResourceWithImportState = &CamundaClusterClientResource{}

type camundaClusterClientData struct {
	Id        types.String `tfsdk:"id"`
	ClusterId types.String `tfsdk:"cluster_id"`
	Name      types.String `tfsdk:"name"`
	Secret    types.String `tfsdk:"secret"`

	ZeebeAddress                types.String `tfsdk:"zeebe_address"`
	ZeebeClientId               types.String `tfsdk:"zeebe_client_id"`
	ZeebeAuthorizationServerUrl types.String `tfsdk:"zeebe_authorization_server_url"`
}

type CamundaClusterClientResource struct {
	provider *CamundaCloudProvider
}

func NewCamundaClusterClientResource() resource.Resource {
	return &CamundaClusterClientResource{}
}

func (r *CamundaClusterClientResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_client"
}

func (r *CamundaClusterClientResource) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Manage a cluster client on Camunda SaaS",

		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Computed:            true,
				MarkdownDescription: "Cluster Client ID",
				PlanModifiers: tfsdk.AttributePlanModifiers{
					resource.UseStateForUnknown(),
				},
				Type: types.StringType,
			},
			"cluster_id": {
				MarkdownDescription: "Cluster ID",
				Required:            true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					resource.UseStateForUnknown(),
				},
				Type: types.StringType,
			},
			"name": {
				MarkdownDescription: "The name of the cluster client",
				Required:            true,
				Type:                types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					resource.RequiresReplace(),
				},
				Validators: []tfsdk.AttributeValidator{
					validators.StringLengthBetweenValidator{Min: 1, Max: 50},
					validators.StringNoSpacesValidator{},
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
					resource.RequiresReplace(),
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

func (r *CamundaClusterClientResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CamundaClusterClientResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
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
			"Unable to create cluster client",
			fmt.Sprintf("Unable to create cluster client, got error: %s",
				err.(*console.GenericOpenAPIError).Body()),
		)
		return
	}

	data.Id = types.String{Value: inline.Uuid}
	data.ZeebeClientId = types.String{Value: inline.ClientId}
	data.Secret = types.String{Value: inline.ClientSecret}

	clientResp, _, err := r.provider.client.ClientsApi.
		GetClient(ctx, data.ClusterId.Value, inline.ClientId).
		Execute()

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to fetch client details",
			fmt.Sprintf("Unable to fetch client details, got error got error: %s",
				err.(*console.GenericOpenAPIError).Body()))
		return
	}

	if clientResp != nil {
		data.ZeebeAddress = types.String{Value: clientResp.ZEEBE_ADDRESS}
		data.ZeebeAuthorizationServerUrl = types.String{Value: clientResp.ZEEBE_AUTHORIZATION_SERVER_URL}
	}

	tflog.Info(ctx, "Camunda cluster client created", map[string]interface{}{
		"Id": data.Id,
	})

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaClusterClientResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
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

func (r *CamundaClusterClientResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data camundaClusterClientData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaClusterClientResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
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

func (r *CamundaClusterClientResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
