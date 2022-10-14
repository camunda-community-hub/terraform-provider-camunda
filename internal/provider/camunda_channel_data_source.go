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

// {
// 	"channels": [
// 			{
// 					"allowedGenerations": [
// 							{
// 									"name": "Zeebe 8.1.0-alpha1",
// 									"uuid": "c1f79896-8d0c-41d0-b8c5-0175157d32de"
// 							}
// 					],
// 					"defaultGeneration": {
// 							"name": "Zeebe 8.1.0-alpha1",
// 							"uuid": "c1f79896-8d0c-41d0-b8c5-0175157d32de"
// 					},
// 					"name": "Alpha",
// 					"uuid": "c767585c-eccc-4762-be78-3bfcd562ee1e"
// 			},
// 			{
// 					"allowedGenerations": [
// 							{
// 									"name": "Zeebe 8.0.2",
// 									"uuid": "edf8342a-ebeb-44f7-9280-356e9c36a1e2"
// 							}
// 					],
// 					"defaultGeneration": {
// 							"name": "Zeebe 8.0.2",
// 							"uuid": "edf8342a-ebeb-44f7-9280-356e9c36a1e2"
// 					},
// 					"name": "Stable",
// 					"uuid": "6bdf0d1c-3d5a-4df6-8d03-762682964d85"
// 			}
// 	],
// 	"clusterPlanTypes": [
// 			{
// 					"name": "Trial Package",
// 					"region": {
// 							"name": "Europe West",
// 							"uuid": "2f6470f9-77ec-4be5-9cdc-3231caf683ec"
// 					},
// 					"uuid": "231932af-0223-4b60-9961-fe4f71800760"
// 			}
// 	],
// 	"regions": [
// 			{
// 					"name": "Europe West",
// 					"uuid": "2f6470f9-77ec-4be5-9cdc-3231caf683ec"
// 			}
// 	]
// }

func (d *CamundaChannelDataSource) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "channel data source",

		Attributes: map[string]tfsdk.Attribute{
			"id": {
				MarkdownDescription: "The ID of the channel",
				Type:                types.StringType,
				Computed:            true,
			},
			"name": {
				MarkdownDescription: "The name of the channel",
				Type:                types.StringType,
				Required:            true,
			},

			"default_generation_id": {
				MarkdownDescription: "The ID of the default generation for this channel",
				Type:                types.StringType,
				Computed:            true,
			},

			"default_generation_name": {
				MarkdownDescription: "The name of the default generation for this channel",
				Type:                types.StringType,
				Computed:            true,
			},

			// https://github.com/hashicorp/terraform-plugin-framework/issues/191
			// "default_generation": {
			// 	MarkdownDescription: "The default generation of this channel",
			// 	Computed:            true,
			// 	Attributes: tfsdk.SingleNestedAttributes(
			// 		map[string]tfsdk.Attribute{
			// 			"name": {
			// 				Computed: true,
			// 				Type:     types.StringType,
			// 			},
			// 			"id": {
			// 				Computed: true,
			// 				Type:     types.StringType,
			// 			},
			// 		},
			// 	),
			// },
		},
	}, nil
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
			fmt.Sprintf("Expected *incidentio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
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
		if channel.Name == data.Name.Value {
			data.Id = types.String{Value: channel.Uuid}
			data.Name = types.String{Value: channel.Name}
			data.DefaultGenerationId = types.String{Value: channel.DefaultGeneration.Uuid}
			data.DefaultGenerationName = types.String{Value: channel.DefaultGeneration.Name}

			diags = resp.State.Set(ctx, &data)
			resp.Diagnostics.Append(diags...)

			return
		}
	}

	resp.Diagnostics.AddError(
		"Client Error",
		fmt.Sprintf("Camunda Cloud channel '%s' not founds", data.Name.Value),
	)
}
