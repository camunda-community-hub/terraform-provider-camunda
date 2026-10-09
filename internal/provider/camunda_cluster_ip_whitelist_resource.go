package provider

import (
	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/camunda-community-hub/terraform-provider-camunda/internal/validators"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type camundaClusterIPWhitelistData struct {
	Id          types.String       `tfsdk:"id"`
	ClusterID   types.String       `tfsdk:"cluster_id"`
	IPWhitelist []ipWhitelistModel `tfsdk:"ip_whitelist"`
}

type ipWhitelistModel struct {
	IP          types.String `tfsdk:"ip"`
	Description types.String `tfsdk:"description"`
}

func NewCamundaClusterIPWhitelistResource() resource.Resource {
	return &managedResource[camundaClusterIPWhitelistData]{
		typeName: "_cluster_ip_whitelist",
		noun:     "cluster IP whitelist",
		schema:   clusterIPWhitelistSchema,
		describe: func(d camundaClusterIPWhitelistData) string { return "cluster " + d.ClusterID.ValueString() },
		create: func(op *op, plan camundaClusterIPWhitelistData) (camundaClusterIPWhitelistData, error) {
			plan.Id = plan.ClusterID
			return plan, op.client.SetIPAllowlist(op.ctx, plan.ClusterID.ValueString(), ipAllowlistFromState(plan))
		},
		read: readClusterIPWhitelist,
		update: func(op *op, prior, plan camundaClusterIPWhitelistData) (camundaClusterIPWhitelistData, error) {
			return plan, op.client.SetIPAllowlist(op.ctx, plan.ClusterID.ValueString(), ipAllowlistFromState(plan))
		},
		delete: func(op *op, prior camundaClusterIPWhitelistData) error {
			// Clearing the allowlist removes all IP restrictions from the cluster.
			return op.client.SetIPAllowlist(op.ctx, prior.ClusterID.ValueString(), nil)
		},
		importID: func(clusterID string) (camundaClusterIPWhitelistData, error) {
			return camundaClusterIPWhitelistData{Id: types.StringValue(clusterID)}, nil
		},
	}
}

func clusterIPWhitelistSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manage IP whitelists of a Camunda cluster",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "Cluster ID",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
		Blocks: map[string]schema.Block{
			"ip_whitelist": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"description": schema.StringAttribute{
							MarkdownDescription: "A short description for this IP whitelist.",
							Optional:            true,
							Default:             stringdefault.StaticString(""),
							Computed:            true,
						},
						"ip": schema.StringAttribute{
							MarkdownDescription: "The IP address/network to whitelist. Must be a valid IPv4 address/network (such as `10.0.0.1` or `172.42.0.0/24`)",
							Required:            true,
							Validators: []validator.String{
								validators.IsIPNetwork{},
							},
						},
					},
				},
			},
		},
	}
}

func readClusterIPWhitelist(op *op, prior camundaClusterIPWhitelistData) (camundaClusterIPWhitelistData, error) {
	allowlist, err := op.client.GetIPAllowlist(op.ctx, prior.Id.ValueString())
	if err != nil {
		return prior, err
	}

	prior.IPWhitelist = []ipWhitelistModel{}
	for _, item := range allowlist {
		prior.IPWhitelist = append(prior.IPWhitelist, ipWhitelistModel{
			IP:          types.StringValue(item.Ip),
			Description: types.StringValue(item.Description),
		})
	}
	// An import only sets the id, which is the cluster ID.
	prior.ClusterID = prior.Id

	return prior, nil
}

func ipAllowlistFromState(data camundaClusterIPWhitelistData) []console.ClusterIpallowlistInner {
	entries := []console.ClusterIpallowlistInner{}
	for _, item := range data.IPWhitelist {
		entries = append(entries, *console.NewClusterIpallowlistInner(
			item.Description.ValueString(),
			item.IP.ValueString(),
		))
	}
	return entries
}
