package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterClientDataSourceData struct {
	Id                          types.String `tfsdk:"id"`
	ClusterId                   types.String `tfsdk:"cluster_id"`
	Name                        types.String `tfsdk:"name"`
	Scopes                      types.Set    `tfsdk:"scopes"`
	ZeebeAddress                types.String `tfsdk:"zeebe_address"`
	ZeebeClientId               types.String `tfsdk:"zeebe_client_id"`
	ZeebeAuthorizationServerUrl types.String `tfsdk:"zeebe_authorization_server_url"`
}

type clusterClientsDataSourceData struct {
	ClusterId types.String                  `tfsdk:"cluster_id"`
	Clients   []clusterClientDataSourceData `tfsdk:"clients"`
}

func newClusterClientDataSourceData(clusterID string, c clusterClient) clusterClientDataSourceData {
	return clusterClientDataSourceData{
		Id:                          types.StringValue(clusterID + "/" + c.ClientID),
		ClusterId:                   types.StringValue(clusterID),
		Name:                        types.StringValue(c.Name),
		Scopes:                      stringSetValue(c.Scopes),
		ZeebeAddress:                types.StringValue(c.ZeebeAddress),
		ZeebeClientId:               types.StringValue(c.ClientID),
		ZeebeAuthorizationServerUrl: types.StringValue(c.AuthorizationServerURL),
	}
}

func NewCamundaClusterClientDataSource() datasource.DataSource {
	return &managedDataSource[clusterClientDataSourceData]{
		typeName: "_cluster_client",
		noun:     "cluster client",
		schema: func() schema.Schema {
			attributes := clusterClientComputedAttributes()
			attributes["id"] = schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster Client ID, in the form `<cluster_id>/<zeebe_client_id>`",
			}
			attributes["cluster_id"] = schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster ID",
			}
			attributes["zeebe_client_id"] = schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zeebe Client Id",
			}
			return schema.Schema{
				MarkdownDescription: "Read a client of a cluster, with the details needed to connect to it. The client secret is only returned when the client is created, so it is not available here.",
				Attributes:          attributes,
			}
		},
		read: func(op *op, config clusterClientDataSourceData) (clusterClientDataSourceData, error) {
			clusterID := config.ClusterId.ValueString()
			clientID := config.ZeebeClientId.ValueString()
			found, err := op.client.GetClusterClient(op.ctx, clusterID, clientID)
			if err != nil {
				return config, fmt.Errorf("client %q in cluster %s: %w", clientID, clusterID, err)
			}
			return newClusterClientDataSourceData(clusterID, *found), nil
		},
	}
}

func NewCamundaClusterClientsDataSource() datasource.DataSource {
	return &managedDataSource[clusterClientsDataSourceData]{
		typeName: "_cluster_clients",
		noun:     "cluster clients",
		schema: func() schema.Schema {
			attributes := clusterClientComputedAttributes()
			attributes["id"] = schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster Client ID, in the form `<cluster_id>/<zeebe_client_id>`",
			}
			attributes["cluster_id"] = schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster ID",
			}
			attributes["zeebe_client_id"] = schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Zeebe Client Id",
			}
			return schema.Schema{
				MarkdownDescription: "Read all clients of a cluster, with the details needed to connect to the cluster. Client secrets are only returned when a client is created, so they are not available here.",
				Attributes: map[string]schema.Attribute{
					"cluster_id": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Cluster ID",
					},
					"clients": schema.ListNestedAttribute{
						Computed:            true,
						MarkdownDescription: "The clients of the cluster.",
						NestedObject:        schema.NestedAttributeObject{Attributes: attributes},
					},
				},
			}
		},
		read: func(op *op, config clusterClientsDataSourceData) (clusterClientsDataSourceData, error) {
			clusterID := config.ClusterId.ValueString()
			listed, err := op.client.ListClusterClients(op.ctx, clusterID)
			if err != nil {
				return config, err
			}

			config.Clients = make([]clusterClientDataSourceData, 0, len(listed))
			for _, c := range listed {
				config.Clients = append(config.Clients, newClusterClientDataSourceData(clusterID, c))
			}
			return config, nil
		},
	}
}

// clusterClientComputedAttributes are the attributes both cluster client data
// sources report; the callers add the identifying ones, which differ in
// whether they are required or computed.
func clusterClientComputedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The name of the client",
		},
		"scopes": schema.SetAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "The scopes the client has",
		},
		"zeebe_address": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Zeebe Address",
		},
		"zeebe_authorization_server_url": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Zeebe Authorization Server URL",
		},
	}
}

// stringSetValue is a Terraform set of the strings, never null.
func stringSetValue(values []string) types.Set {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	return types.SetValueMust(types.StringType, elements)
}
