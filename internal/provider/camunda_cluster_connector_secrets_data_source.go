package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterConnectorSecretsDataSourceData struct {
	ClusterId types.String   `tfsdk:"cluster_id"`
	Names     []types.String `tfsdk:"names"`
}

func NewCamundaClusterConnectorSecretsDataSource() datasource.DataSource {
	return &managedDataSource[clusterConnectorSecretsDataSourceData]{
		typeName: "_cluster_connector_secrets",
		noun:     "cluster connector secrets",
		schema: func() schema.Schema {
			return schema.Schema{
				MarkdownDescription: "List the names of the connector secrets of a cluster. The values are deliberately not exposed, so they don't end up in state.",

				Attributes: map[string]schema.Attribute{
					"cluster_id": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Cluster ID",
					},
					"names": schema.ListAttribute{
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "The names of the connector secrets, sorted alphabetically.",
					},
				},
			}
		},
		read: func(op *op, config clusterConnectorSecretsDataSourceData) (clusterConnectorSecretsDataSourceData, error) {
			names, err := op.client.ListSecretNames(op.ctx, config.ClusterId.ValueString())
			if err != nil {
				return config, err
			}
			config.Names = stringValues(names)
			return config, nil
		},
	}
}
