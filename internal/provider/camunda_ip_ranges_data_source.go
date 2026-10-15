package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ipRangesDataSourceData struct {
	Id         types.String   `tfsdk:"id"`
	WebModeler []types.String `tfsdk:"web_modeler"`
}

func NewCamundaIPRangesDataSource() datasource.DataSource {
	return &managedDataSource[ipRangesDataSourceData]{
		typeName: "_ip_ranges",
		noun:     "IP ranges",
		schema: func() schema.Schema {
			return schema.Schema{
				MarkdownDescription: "IP ranges Camunda SaaS services connect from. Use them to allow those services through a firewall or IP allowlist.",

				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Always `ip-ranges`.",
					},
					"web_modeler": schema.ListAttribute{
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "IP ranges Web Modeler connects from, for example when calling a connector's endpoint.",
					},
				},
			}
		},
		read: func(op *op, config ipRangesDataSourceData) (ipRangesDataSourceData, error) {
			ranges, err := op.client.GetIPRanges(op.ctx)
			if err != nil {
				return config, err
			}

			config.Id = types.StringValue("ip-ranges")
			config.WebModeler = stringValues(ranges.WebModeler)
			return config, nil
		},
	}
}

// stringValues converts plain strings to Terraform values; nil becomes an
// empty slice so the attribute is an empty list rather than null.
func stringValues(values []string) []types.String {
	converted := make([]types.String, 0, len(values))
	for _, value := range values {
		converted = append(converted, types.StringValue(value))
	}
	return converted
}
