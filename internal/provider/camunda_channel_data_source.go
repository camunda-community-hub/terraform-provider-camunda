package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	AllowedGenerations    types.List   `tfsdk:"allowed_generations"`
	AllowedGenerationIds  types.Map    `tfsdk:"allowed_generation_ids"`

	// https://github.com/hashicorp/terraform-plugin-framework/issues/191
	// DefaultGeneration generation   `tfsdk:"default_generation"`
}

type CamundaChannelDataSource struct {
	client *consoleClient
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
			"allowed_generations": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The ID of the generation",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the generation",
							Computed:            true,
						},
					},
				},
				MarkdownDescription: "The allowed generations for this channel",
				Computed:            true,
			},
			"allowed_generation_ids": schema.MapAttribute{
				MarkdownDescription: "The IDs of the allowed generations for this channel, keyed by generation name. " +
					"A cluster on an older generation may run one that is no longer allowed and so isn't listed here; " +
					"after importing such a cluster, its `current_generation` attribute holds the generation ID it runs.",
				ElementType: types.StringType,
				Computed:    true,
			},
		},
	}
}

func (d *CamundaChannelDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	client, diags := consoleClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	d.client = client
}

type Generation struct {
	Id   string `tfsdk:"id"`
	Name string `tfsdk:"name"`
}

func (d *CamundaChannelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data channelDataSourceData

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

	for _, channel := range params.Channels {
		if channel.Name == data.Name.ValueString() {

			data.Id = types.StringValue(channel.Uuid)
			data.Name = types.StringValue(channel.Name)
			data.DefaultGenerationId = types.StringValue(channel.DefaultGeneration.Uuid)
			data.DefaultGenerationName = types.StringValue(channel.DefaultGeneration.Name)

			var allowedGenerations []Generation
			allowedGenerationIds := map[string]string{}
			for _, generation := range channel.AllowedGenerations {
				allowedGenerations = append(allowedGenerations, Generation{
					Id:   generation.Uuid,
					Name: generation.Name,
				})
				allowedGenerationIds[generation.Name] = generation.Uuid
			}

			allowedGenerationsTF, diags := types.ListValueFrom(ctx, types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"id":   types.StringType,
					"name": types.StringType,
				},
			}, allowedGenerations)
			resp.Diagnostics.Append(diags...)
			data.AllowedGenerations = allowedGenerationsTF

			allowedGenerationIdsTF, diags := types.MapValueFrom(ctx, types.StringType, allowedGenerationIds)
			resp.Diagnostics.Append(diags...)
			data.AllowedGenerationIds = allowedGenerationIdsTF

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
