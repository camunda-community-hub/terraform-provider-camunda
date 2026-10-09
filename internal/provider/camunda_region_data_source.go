package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &CamundaRegionDataSource{}

type regionDataSourceData struct {
	Id   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

type CamundaRegionDataSource struct {
	client *consoleClient
}

func NewCamundaRegionDataSource() datasource.DataSource {
	return &CamundaRegionDataSource{}
}

func (d *CamundaRegionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_region"
}

func (d *CamundaRegionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "region data source",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the region",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the region",
				Required:            true,
			},
		},
	}
}

func (d *CamundaRegionDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	client, diags := consoleClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	d.client = client
}

func (d *CamundaRegionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data regionDataSourceData

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	params, err := d.client.GetParameters(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read parameters, got error: %s", err),
		)
		return
	}

	wantedRegion := data.Name.ValueString()

	for _, region := range params.Regions {
		if region.Name == wantedRegion {
			data.Id = types.StringValue(region.ID)
			data.Name = types.StringValue(region.Name)

			diags = resp.State.Set(ctx, &data)
			resp.Diagnostics.Append(diags...)

			return
		}
	}

	resp.Diagnostics.AddError(
		"Client Error",
		fmt.Sprintf("Camunda Cloud region '%s' not found.", wantedRegion),
	)
}
