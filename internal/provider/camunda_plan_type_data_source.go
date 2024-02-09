package provider

import (
	"context"
	"fmt"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &CamundaClusterPlanTypeDataSource{}

type clusterPlanTypeDataSourceData struct {
	Id   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

type CamundaClusterPlanTypeDataSource struct {
	provider *CamundaCloudProvider
}

func NewCamundaClusterPlanTypeDataSource() datasource.DataSource {
	return &CamundaClusterPlanTypeDataSource{}
}

func (d *CamundaClusterPlanTypeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_plan_type"
}

func (d *CamundaClusterPlanTypeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "clusterPlanType data source",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the clusterPlanType",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the clusterPlanType",
				Required:            true,
			},
		},
	}
}

func (d *CamundaClusterPlanTypeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	provider, ok := req.ProviderData.(*CamundaCloudProvider)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *CamundaCloudProvider Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.provider = provider
}

func (d *CamundaClusterPlanTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data clusterPlanTypeDataSourceData

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, d.provider.accessToken)
	params, _, err := d.provider.client.DefaultAPI.GetParameters(ctx).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read parameters, got error: %s", formatClientError(err)),
		)
		return
	}

	for _, clusterPlanType := range params.ClusterPlanTypes {
		if clusterPlanType.Name == data.Name.ValueString() {
			data.Id = types.StringValue(clusterPlanType.Uuid)
			data.Name = types.StringValue(clusterPlanType.Name)

			diags = resp.State.Set(ctx, &data)
			resp.Diagnostics.Append(diags...)

			return
		}
	}

	resp.Diagnostics.AddError(
		"Client Error",
		fmt.Sprintf("Camunda Cloud clusterPlanType '%s' not found.", data.Name.ValueString()),
	)
}
