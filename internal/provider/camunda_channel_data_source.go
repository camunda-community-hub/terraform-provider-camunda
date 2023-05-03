package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	console "github.com/sijoma/console-customer-api-go"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &CamundaChannelDataSource{}

// type generation struct {
// 	Id   types.String `tfsdk:"id"`
// 	Name types.String `tfsdk:"name"`
// }

type channelDataSourceData struct {
	Id                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	DefaultGenerationName types.String `tfsdk:"default_generation_name"`
	DefaultGenerationId   types.String `tfsdk:"default_generation_id"`

	// https://github.com/hashicorp/terraform-plugin-framework/issues/191
	// DefaultGeneration generation   `tfsdk:"default_generation"`
}

type CamundaChannelDataSource struct {
	provider *CamundaCloudProvider
}

func NewCamundaChannelDataSource() datasource.DataSource {
	return &CamundaChannelDataSource{}
}

func (d *CamundaChannelDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_channel"
}

func (d *CamundaChannelDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "channel data source",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the channel",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the channel",
				Required:            true,
			},

			"default_generation_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the default generation for this channel",
				Computed:            true,
			},

			"default_generation_name": schema.StringAttribute{
				MarkdownDescription: "The name of the default generation for this channel",
				Computed:            true,
			},
		},
	}
}

func (d *CamundaChannelDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	d.provider = provider
}

func (d *CamundaChannelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data channelDataSourceData

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
			fmt.Sprintf("Unable to read parameters, got error: %s", err.(*console.GenericOpenAPIError).Body()),
		)
		return
	}

	for _, channel := range params.Channels {
		if channel.Name == data.Name.ValueString() {
			data.Id = types.StringValue(channel.Uuid)
			data.Name = types.StringValue(channel.Name)
			data.DefaultGenerationId = types.StringValue(channel.DefaultGeneration.Uuid)
			data.DefaultGenerationName = types.StringValue(channel.DefaultGeneration.Name)

			diags = resp.State.Set(ctx, &data)
			resp.Diagnostics.Append(diags...)

			return
		}
	}

	resp.Diagnostics.AddError(
		"Client Error",
		fmt.Sprintf("Camunda Cloud channel '%s' not found.", data.Name.ValueString()),
	)
}
