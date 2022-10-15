package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	console "github.com/sijoma/console-customer-api-go"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &CamundaClusterPlanTypeDataSource{}

type clusterPlanTypeDataSourceData struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	RegionName types.String `tfsdk:"region_name"`
	RegionID   types.String `tfsdk:"region_id"`
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

func (d *CamundaClusterPlanTypeDataSource) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "clusterPlanType data source",

		Attributes: map[string]tfsdk.Attribute{
			"id": {
				MarkdownDescription: "The ID of the clusterPlanType",
				Type:                types.StringType,
				Computed:            true,
			},
			"name": {
				MarkdownDescription: "The name of the clusterPlanType",
				Type:                types.StringType,
				Required:            true,
			},

			"region_id": {
				MarkdownDescription: "The ID of the region for this clusterPlanType",
				Type:                types.StringType,
				Computed:            true,
			},

			"region_name": {
				MarkdownDescription: "The name of the for this clusterPlanType",
				Type:                types.StringType,
				Computed:            true,
			},
		},
	}, nil
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
	params, _, err := d.provider.client.ClustersApi.GetParameters(ctx).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read parameters, got error: %s", err.(console.GenericOpenAPIError).Body()),
		)
		return
	}

	for _, clusterPlanType := range params.ClusterPlanTypes {
		if clusterPlanType.Name == data.Name.Value {
			data.Id = types.String{Value: clusterPlanType.Uuid}
			data.Name = types.String{Value: clusterPlanType.Name}
			data.RegionID = types.String{Value: clusterPlanType.Region.Uuid}
			data.RegionName = types.String{Value: clusterPlanType.Region.Name}

			diags = resp.State.Set(ctx, &data)
			resp.Diagnostics.Append(diags...)

			return
		}
	}

	resp.Diagnostics.AddError(
		"Client Error",
		fmt.Sprintf("Camunda Cloud clusterPlanType '%s' not found", data.Name.Value),
	)
	return
}
