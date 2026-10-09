package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type camundaClusterConnectorSecret struct {
	ClusterId types.String `tfsdk:"cluster_id"`
	Name      types.String `tfsdk:"name"`
	Value     types.String `tfsdk:"value"`
}

func NewCamundaClusterConnectorSecretResource() resource.Resource {
	return &managedResource[camundaClusterConnectorSecret]{
		typeName: "_cluster_connector_secret",
		noun:     "cluster connector secret",
		schema:   clusterConnectorSecretSchema,
		describe: func(d camundaClusterConnectorSecret) string {
			return d.Name.ValueString() + " in cluster " + d.ClusterId.ValueString()
		},
		create: func(op *op, plan camundaClusterConnectorSecret) (camundaClusterConnectorSecret, error) {
			return plan, op.client.CreateSecret(op.ctx, plan.ClusterId.ValueString(), plan.Name.ValueString(), plan.Value.ValueString())
		},
		read: func(op *op, prior camundaClusterConnectorSecret) (camundaClusterConnectorSecret, error) {
			value, err := op.client.GetSecret(op.ctx, prior.ClusterId.ValueString(), prior.Name.ValueString())
			prior.Value = types.StringValue(value)
			return prior, err
		},
		delete: func(op *op, prior camundaClusterConnectorSecret) error {
			return op.client.DeleteSecret(op.ctx, prior.ClusterId.ValueString(), prior.Name.ValueString())
		},
		importID: importClusterConnectorSecret,
	}
}

func clusterConnectorSecretSchema() schema.Schema {
	return schema.Schema{
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

// importClusterConnectorSecret takes "<cluster_id>/<secret_name>".
func importClusterConnectorSecret(id string) (camundaClusterConnectorSecret, error) {
	clusterID, name, diags := splitImportID(id, "<cluster_id>/<secret_name>")
	if diags.HasError() {
		return camundaClusterConnectorSecret{}, diagsError(diags)
	}
	return camundaClusterConnectorSecret{
		ClusterId: types.StringValue(clusterID),
		Name:      types.StringValue(name),
	}, nil
}
