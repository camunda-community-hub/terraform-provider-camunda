package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &CamundaClusterConnectorSecretResource{}
var _ resource.ResourceWithImportState = &CamundaClusterConnectorSecretResource{}

type camundaClusterConnectorSecret struct {
	ClusterId types.String `tfsdk:"cluster_id"`
	Name      types.String `tfsdk:"name"`
	Value     types.String `tfsdk:"value"`
}

type CamundaClusterConnectorSecretResource struct {
	provider *CamundaCloudProvider
}

func NewCamundaClusterConnectorSecretResource() resource.Resource {
	return &CamundaClusterConnectorSecretResource{}
}

func (r *CamundaClusterConnectorSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_connector_secret"
}

func (r *CamundaClusterConnectorSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a cluster connector secret on Camunda SaaS.",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster Connector Secret Name",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 50),
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[^ ]+$`),
						"must not contain space characters",
					),
				},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster ID",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "The value of the connector secret",
				Required:            true,
				// Todo: Its actually also possible to update the secret value in-place. Not sure whether
				// that just needs a different implementation or the API is not documented in the spec.
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Sensitive:     true,
			},
		},
	}
}

func (r *CamundaClusterConnectorSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	provider, ok := req.ProviderData.(*CamundaCloudProvider)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *CamundaCloudProvider, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.provider = provider
}

func (r *CamundaClusterConnectorSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data camundaClusterConnectorSecret

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	newClusterConnectorSecretConfiguration := console.CreateSecretBody{
		SecretName:  data.Name.ValueString(),
		SecretValue: data.Value.ValueString(),
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	response, err := r.provider.client.DefaultAPI.
		CreateSecret(ctx, data.ClusterId.ValueString()).
		CreateSecretBody(newClusterConnectorSecretConfiguration).
		Execute()

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cluster connector secret",
			fmt.Sprintf("Unable to create cluster connector secret, got error: %s",
				formatClientError(err)),
		)
		return
	}

	if response.StatusCode >= 300 {
		resp.Diagnostics.AddError(
			"Unable to create cluster connector secret",
			fmt.Sprintf("Unable to create cluster connector secret, got status: %s", response.Status))
		return
	}

	tflog.Info(ctx, "Camunda cluster connector secret created", map[string]interface{}{
		"Name":      data.Name,
		"ClusterId": data.ClusterId,
	})

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaClusterConnectorSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data camundaClusterConnectorSecret

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	secrets, response, err := r.provider.client.DefaultAPI.GetSecrets(ctx, data.ClusterId.ValueString()).Execute()
	if err != nil && response.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Connector Secret Error",
			fmt.Sprintf("Unable to read cluster connector secrets Name=%s, ClusterID=%s, got error: %s",
				data.Name.ValueString(), data.ClusterId.ValueString(), formatClientError(err)),
		)
		return
	}

	// Find secrets name in list of secrets
	// We also check if the value changed
	found := false
	for key, value := range secrets {
		if key == data.Name.ValueString() {
			found = true
			data.Value = types.StringValue(value)
			break
		}
	}

	// Remove secret from state
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaClusterConnectorSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data camundaClusterConnectorSecret

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaClusterConnectorSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data camundaClusterConnectorSecret

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	_, err := r.provider.client.DefaultAPI.DeleteSecret(ctx, data.ClusterId.ValueString(), data.Name.ValueString()).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Connector Secret Error",
			fmt.Sprintf("Unable to delete cluster connector secret Name=%s, ClusterId=%s, got error: %s",
				data.Name.ValueString(), data.ClusterId.ValueString(), formatClientError(err)),
		)
		return
	}
}

func (r *CamundaClusterConnectorSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
