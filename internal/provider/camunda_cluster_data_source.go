package provider

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterDataSourceData struct {
	Id               types.String          `tfsdk:"id"`
	Name             types.String          `tfsdk:"name"`
	Description      types.String          `tfsdk:"description"`
	Channel          types.String          `tfsdk:"channel"`
	Region           types.String          `tfsdk:"region"`
	PlanType         types.String          `tfsdk:"plan_type"`
	Generation       types.String          `tfsdk:"generation"`
	AutoUpdate       types.Bool            `tfsdk:"auto_update"`
	BackupRegion     types.String          `tfsdk:"backup_region"`
	Encryption       types.String          `tfsdk:"encryption"`
	EncryptionStatus types.String          `tfsdk:"encryption_status"`
	OwnerID          types.String          `tfsdk:"owner_id"`
	Created          types.String          `tfsdk:"created"`
	Labels           []types.String        `tfsdk:"labels"`
	IPAllowlist      []ipAllowlistDataItem `tfsdk:"ip_allowlist"`
	Status           types.String          `tfsdk:"status"`
	Links            *clusterLinksModel    `tfsdk:"links"`
}

type ipAllowlistDataItem struct {
	IP          types.String `tfsdk:"ip"`
	Description types.String `tfsdk:"description"`
}

type clustersDataSourceData struct {
	Id       types.String            `tfsdk:"id"`
	Clusters []clusterDataSourceData `tfsdk:"clusters"`
}

func newClusterDataSourceData(cl cluster) clusterDataSourceData {
	allowlist := make([]ipAllowlistDataItem, 0, len(cl.IPAllowlist))
	for _, entry := range cl.IPAllowlist {
		allowlist = append(allowlist, ipAllowlistDataItem{
			IP:          types.StringValue(entry.IP),
			Description: types.StringValue(entry.Description),
		})
	}

	return clusterDataSourceData{
		Id:               types.StringValue(cl.ID),
		Name:             types.StringValue(cl.Name),
		Description:      nullIfEmpty(cl.Description),
		Channel:          types.StringValue(cl.ChannelID),
		Region:           types.StringValue(cl.RegionID),
		PlanType:         types.StringValue(cl.PlanTypeID),
		Generation:       types.StringValue(cl.GenerationID),
		AutoUpdate:       types.BoolValue(cl.AutoUpdate),
		BackupRegion:     nullIfEmpty(cl.BackupRegionID),
		Encryption:       nullIfEmpty(cl.Encryption),
		EncryptionStatus: nullIfEmpty(cl.EncryptionStatus),
		OwnerID:          types.StringValue(cl.OwnerID),
		Created:          types.StringValue(cl.Created.Format(time.RFC3339)),
		Labels:           stringValues(cl.Labels),
		IPAllowlist:      allowlist,
		Status:           types.StringValue(cl.Status),
		Links:            newClusterLinksModel(cl.Links),
	}
}

func NewCamundaClusterDataSource() datasource.DataSource {
	return &managedDataSource[clusterDataSourceData]{
		typeName: "_cluster",
		noun:     "cluster",
		schema: func() schema.Schema {
			attributes := clusterDataSourceAttributes()
			attributes["id"] = schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster ID",
			}
			return schema.Schema{
				MarkdownDescription: "Read a cluster of the organization. Fails if the cluster doesn't exist.",
				Attributes:          attributes,
			}
		},
		read: func(op *op, config clusterDataSourceData) (clusterDataSourceData, error) {
			found, err := op.client.GetCluster(op.ctx, config.Id.ValueString())
			if err != nil {
				return config, err
			}
			return newClusterDataSourceData(*found), nil
		},
	}
}

func NewCamundaClustersDataSource() datasource.DataSource {
	return &managedDataSource[clustersDataSourceData]{
		typeName: "_clusters",
		noun:     "clusters",
		schema: func() schema.Schema {
			attributes := clusterDataSourceAttributes()
			attributes["id"] = schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster ID",
			}
			return schema.Schema{
				MarkdownDescription: "Read all clusters of the organization.",
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Always `clusters`.",
					},
					"clusters": schema.ListNestedAttribute{
						Computed:            true,
						MarkdownDescription: "The clusters of the organization.",
						NestedObject:        schema.NestedAttributeObject{Attributes: attributes},
					},
				},
			}
		},
		read: func(op *op, config clustersDataSourceData) (clustersDataSourceData, error) {
			listed, err := op.client.ListClusters(op.ctx)
			if err != nil {
				return config, err
			}

			config.Id = types.StringValue("clusters")
			config.Clusters = make([]clusterDataSourceData, 0, len(listed))
			for _, cl := range listed {
				config.Clusters = append(config.Clusters, newClusterDataSourceData(cl))
			}
			return config, nil
		},
	}
}

// clusterDataSourceAttributes are the computed attributes both cluster data
// sources report; the callers add id, which one requires and the other reports.
func clusterDataSourceAttributes() map[string]schema.Attribute {
	str := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: description}
	}

	return map[string]schema.Attribute{
		"name":        str("The name of the cluster"),
		"description": str("Description of the cluster. Null if it has none."),
		"channel":     str("ID of the cluster's channel"),
		"region":      str("ID of the cluster's region"),
		"plan_type":   str("ID of the cluster's plan type"),
		"generation":  str("ID of the generation the cluster currently runs"),
		"auto_update": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether Camunda upgrades the cluster automatically.",
		},
		"backup_region":     str("ID of the region that stores the cluster's backups. Null if the cluster has none."),
		"encryption":        str("Type of the key that encrypts the cluster's data: `Software`, `Hardware`, `Provider` or `External`."),
		"encryption_status": str("State of the cluster's encryption. Only reported when `encryption` is `External`."),
		"owner_id":          str("ID of the user or client that owns the cluster."),
		"created":           str("When the cluster was created, as an RFC 3339 timestamp."),
		"labels": schema.ListAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Labels of the cluster, as reported by the Console.",
		},
		"ip_allowlist": schema.SetNestedAttribute{
			Computed:            true,
			MarkdownDescription: "The IP allowlist of the cluster, see `camunda_cluster_ip_whitelist`. Empty if the cluster has none, or if the organization doesn't have the feature or the client may not see it.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"ip":          str("The allowed IP address or network."),
					"description": str("A short description of the entry."),
				},
			},
		},
		"status": str("Status of the cluster, such as `Healthy` or `Suspended`."),
		"links":  clusterLinksAttribute(),
	}
}

func clusterLinksAttribute() schema.Attribute {
	str := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: description}
	}

	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "URLs of the cluster's components. A component the cluster doesn't have has an empty URL.",
		Attributes: map[string]schema.Attribute{
			"zeebe":      str("Zeebe gateway URL."),
			"operate":    str("Operate URL."),
			"tasklist":   str("Tasklist URL."),
			"optimize":   str("Optimize URL."),
			"connectors": str("Connectors URL."),
			"identity":   str("Identity URL."),
			"console":    str("URL of the cluster in Console."),
			"oauth":      str("OAuth token URL."),
		},
	}
}
