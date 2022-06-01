package provider

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ tfsdk.DataSourceType = channelDataSourceType{}
var _ tfsdk.DataSource = channelDataSource{}

type channelDataSourceType struct {
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

func (t channelDataSourceType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "channel data source",

		Attributes: map[string]tfsdk.Attribute{
			"id": {
				MarkdownDescription: "channel identifier",
				Type:                types.StringType,
				Computed:            true,
			},
			"name": {
				MarkdownDescription: "channel identifier",
				Type:                types.StringType,
				Required:            true,
			},
			"default_generation": {
				MarkdownDescription: "The default generation of this channel",
				Attributes: tfsdk.SingleNestedAttributes(
					map[string]tfsdk.Attribute{
						"name": {
							Computed: true,
							Type:     types.StringType,
						},
						"id": {
							Computed: true,
							Type:     types.StringType,
						},
					},
				),
			},
		},
	}, nil
}

func (t channelDataSourceType) NewDataSource(ctx context.Context, in tfsdk.Provider) (tfsdk.DataSource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return channelDataSource{
		provider: provider,
	}, diags
}

type generation struct {
	Id   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

type channelDataSourceData struct {
	Id                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	DefaultGeneration generation   `tfsdk:"default_generation"`
}

type channelDataSource struct {
	provider provider
}

func (d channelDataSource) Read(ctx context.Context, req tfsdk.ReadDataSourceRequest, resp *tfsdk.ReadDataSourceResponse) {
	var data channelDataSourceData

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	log.Printf("got here")

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("got here")

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// channel, err := d.provider.client.Readchannel(...)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read channel, got error: %s", err))
	//     return
	// }

	// For the purposes of this channel code, hardcoding a response value to
	// save into the Terraform state.
	data.Id = types.String{Value: "example-id"}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}
