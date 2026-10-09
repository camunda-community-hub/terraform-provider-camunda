package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

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

func NewCamundaClusterClientResource() resource.Resource {
	return &managedResource[camundaClusterClientData]{
		typeName: "_cluster_client",
		noun:     "cluster client",
		schema:   clusterClientSchema,
		describe: func(d camundaClusterClientData) string {
			return d.Name.ValueString() + " in cluster " + d.ClusterId.ValueString()
		},
		create: createClusterClient,
		read:   readClusterClient,
		delete: func(op *op, prior camundaClusterClientData) error {
			return op.client.DeleteClusterClient(op.ctx, prior.ClusterId.ValueString(), prior.ZeebeClientId.ValueString())
		},
		importID: importClusterClient,
	}
}

func clusterClientSchema() schema.Schema {
	defaultScopes := []attr.Value{}
	for _, scope := range validScopes {
		defaultScopes = append(defaultScopes, types.StringValue(scope))
	}

	return schema.Schema{
		MarkdownDescription: "Manage a cluster client on Camunda SaaS.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster Client ID, in the form `<cluster_id>/<zeebe_client_id>`",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "Cluster ID",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
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

// createClusterClient creates the client and keeps its secret, which the
// API only returns here.
func createClusterClient(op *op, plan camundaClusterClientData) (camundaClusterClientData, error) {
	var scopes []string
	for _, scope := range plan.Scopes {
		scopes = append(scopes, scope.ValueString())
	}

	created, err := op.client.CreateClusterClient(op.ctx, plan.ClusterId.ValueString(), plan.Name.ValueString(), scopes)
	if err != nil {
		return plan, err
	}

	plan.ZeebeClientId = types.StringValue(created.ClientId)
	plan.Secret = types.StringValue(created.ClientSecret)
	return plan, nil
}

func readClusterClient(op *op, prior camundaClusterClientData) (camundaClusterClientData, error) {
	client, err := op.client.GetClusterClient(op.ctx, prior.ClusterId.ValueString(), prior.ZeebeClientId.ValueString())
	if err != nil {
		return prior, err
	}

	prior.Id = types.StringValue(prior.ClusterId.ValueString() + "/" + client.ClientID)
	prior.Name = types.StringValue(client.Name)
	prior.ZeebeClientId = types.StringValue(client.ClientID)
	prior.ZeebeAddress = types.StringValue(client.ZeebeAddress)
	prior.ZeebeAuthorizationServerUrl = types.StringValue(client.AuthorizationServerURL)

	prior.Scopes = []types.String{}
	for _, scope := range client.Scopes {
		prior.Scopes = append(prior.Scopes, types.StringValue(scope))
	}
	return prior, nil
}

// importClusterClient takes "<cluster_id>/<zeebe_client_id>". The client
// secret can't be read back, so it stays empty after an import.
func importClusterClient(id string) (camundaClusterClientData, error) {
	clusterID, clientID, err := splitImportID(id, "<cluster_id>/<zeebe_client_id>")
	if err != nil {
		return camundaClusterClientData{}, err
	}
	return camundaClusterClientData{
		ClusterId:     types.StringValue(clusterID),
		ZeebeClientId: types.StringValue(clientID),
	}, nil
}
