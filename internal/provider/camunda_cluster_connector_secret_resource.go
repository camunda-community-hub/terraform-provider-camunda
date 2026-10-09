package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

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
	client *consoleClient
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
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
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
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
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

	client, diags := consoleClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.client = client
}

func (r *CamundaClusterConnectorSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data camundaClusterConnectorSecret

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.CreateSecret(ctx, data.ClusterId.ValueString(), data.Name.ValueString(), data.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cluster connector secret",
			fmt.Sprintf("Unable to create cluster connector secret, got error: %s", err),
		)
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

	value, err := r.client.GetSecret(ctx, data.ClusterId.ValueString(), data.Name.ValueString())
	if errors.Is(err, errNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Connector Secret Error",
			fmt.Sprintf("Unable to read cluster connector secrets Name=%s, ClusterID=%s, got error: %s",
				data.Name.ValueString(), data.ClusterId.ValueString(), err),
		)
		return
	}

	data.Value = types.StringValue(value)

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaClusterConnectorSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.Append(unexpectedUpdate())
}

func (r *CamundaClusterConnectorSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data camundaClusterConnectorSecret

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSecret(ctx, data.ClusterId.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Connector Secret Error",
			fmt.Sprintf("Unable to delete cluster connector secret Name=%s, ClusterId=%s, got error: %s",
				data.Name.ValueString(), data.ClusterId.ValueString(), err),
		)
		return
	}
}

func (r *CamundaClusterConnectorSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
