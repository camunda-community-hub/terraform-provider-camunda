package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &CamundaClusterClientResource{}
var _ resource.ResourceWithImportState = &CamundaClusterClientResource{}

var validScopes = []string{"Operate", "Optimize", "Tasklist", "Zeebe"}

type camundaClusterClientData struct {
	Id        types.String   `tfsdk:"id"`
	ClusterId types.String   `tfsdk:"cluster_id"`
	Name      types.String   `tfsdk:"name"`
	Secret    types.String   `tfsdk:"secret"`
	Scopes    []types.String `tfsdk:"scopes"`

	ZeebeAddress                types.String `tfsdk:"zeebe_address"`
	ZeebeClientId               types.String `tfsdk:"zeebe_client_id"`
	ZeebeAuthorizationServerUrl types.String `tfsdk:"zeebe_authorization_server_url"`
}

type CamundaClusterClientResource struct {
	client *consoleClient
}

func NewCamundaClusterClientResource() resource.Resource {
	return &CamundaClusterClientResource{}
}

func (r *CamundaClusterClientResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_client"
}

func (r *CamundaClusterClientResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	defaultScopes := []attr.Value{}
	for _, scope := range validScopes {
		defaultScopes = append(defaultScopes, types.StringValue(scope))
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a cluster client on Camunda SaaS.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster Client ID",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "Cluster ID",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster client",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 50),
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[^ ]+$`),
						"must not contain space characters",
					),
				},
			},
			"scopes": schema.SetAttribute{
				ElementType: types.StringType,
				MarkdownDescription: ("The list of scopes the client will be valid for. It defaults to all the scopes, and at least one scope should be specified. Valid values:\n" +
					"  * `Operate`\n" +
					"  * `Optimize`\n" +
					"  * `Tasklist`\n" +
					"  * `Zeebe`\n"),
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
					setplanmodifier.RequiresReplace(),
				},
				Validators: []validator.Set{
					// At least one valid scope must be specified.
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf(validScopes...),
					),
				},
				Default: setdefault.StaticValue(
					types.SetValueMust(
						types.StringType,
						defaultScopes,
					),
				),
			},
			"secret": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The client secret",
				Sensitive:           true,
			},
			"zeebe_address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Zeebe Address",
			},
			"zeebe_client_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Zeebe Client Id",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"zeebe_authorization_server_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Zeebe Authorization Server Url",
			},
		},
	}
}

func (r *CamundaClusterClientResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	client, diags := consoleClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.client = client
}

func (r *CamundaClusterClientResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data camundaClusterClientData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var scopes []string
	for _, scope := range data.Scopes {
		scopes = append(scopes, scope.ValueString())
	}

	inline, err := r.client.CreateClusterClient(ctx, data.ClusterId.ValueString(), data.Name.ValueString(), scopes)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cluster client",
			fmt.Sprintf("Unable to create cluster client, got error: %s", err),
		)
		return
	}

	data.Id = types.StringValue(inline.Uuid)
	data.ZeebeClientId = types.StringValue(inline.ClientId)
	data.Secret = types.StringValue(inline.ClientSecret)

	data.Scopes = []types.String{}
	for _, permission := range inline.Permissions {
		data.Scopes = append(data.Scopes, types.StringValue(permission))
	}

	clientResp, err := r.client.GetClusterClient(ctx, data.ClusterId.ValueString(), inline.ClientId)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to fetch client details",
			fmt.Sprintf("Unable to fetch client details, got error: %s", err))
		return
	}

	data.ZeebeAddress = types.StringValue(clientResp.ZEEBE_ADDRESS)
	data.ZeebeAuthorizationServerUrl = types.StringValue(clientResp.ZEEBE_AUTHORIZATION_SERVER_URL)

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

	client, err := r.client.GetClusterClient(ctx, data.ClusterId.ValueString(), data.ZeebeClientId.ValueString())
	if errors.Is(err, errNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read cluster client ID=%s, got error: %s", data.Id.ValueString(), err),
		)
		return
	}

	data.Name = types.StringValue(client.Name)
	data.ZeebeClientId = types.StringValue(client.ZEEBE_CLIENT_ID)
	data.ZeebeAddress = types.StringValue(client.ZEEBE_ADDRESS)
	data.ZeebeAuthorizationServerUrl = types.StringValue(client.ZEEBE_AUTHORIZATION_SERVER_URL)
	// TODO: implement reading scopes while reading from the API

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

	err := r.client.DeleteClusterClient(ctx, data.ClusterId.ValueString(), data.ZeebeClientId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to delete cluster client ID=%s, got error: %s", data.Id.ValueString(), err),
		)
		return
	}
}

func (r *CamundaClusterClientResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
