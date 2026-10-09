package provider

import (
	"context"
	"errors"
	"fmt"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/camunda-community-hub/terraform-provider-camunda/internal/validators"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &CamundaClusterIPWhiteListResource{}
var _ resource.ResourceWithImportState = &CamundaClusterIPWhiteListResource{}

type camundaClusterIPWhitelistData struct {
	Id          types.String       `tfsdk:"id"`
	ClusterID   types.String       `tfsdk:"cluster_id"`
	IPWhitelist []ipWhitelistModel `tfsdk:"ip_whitelist"`
}

type ipWhitelistModel struct {
	IP          types.String `tfsdk:"ip"`
	Description types.String `tfsdk:"description"`
}

type CamundaClusterIPWhiteListResource struct {
	client *consoleClient
}

func NewCamundaClusterIPWhitelistResource() resource.Resource {
	return &CamundaClusterIPWhiteListResource{}
}

func (r *CamundaClusterIPWhiteListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_ip_whitelist"
}

func (r *CamundaClusterIPWhiteListResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
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

func (r *CamundaClusterIPWhiteListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	client, diags := consoleClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.client = client
}

func (r *CamundaClusterIPWhiteListResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data camundaClusterIPWhitelistData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	clusterId := data.ClusterID.ValueString()

	ipWhitelistPath := path.Root("ip_whitelist")
	err := r.client.SetIPAllowlist(ctx, clusterId, ipAllowlistFromState(data))
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			ipWhitelistPath,
			"Unable to configure IP whitelisting",
			err.Error(),
		)
		return
	}

	data.ClusterID = types.StringValue(clusterId)
	data.Id = types.StringValue(clusterId)
	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)

	diags = resp.State.SetAttribute(ctx, ipWhitelistPath, data.IPWhitelist)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *CamundaClusterIPWhiteListResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data camundaClusterIPWhitelistData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	allowlist, err := r.client.GetIPAllowlist(ctx, data.Id.ValueString())
	if errors.Is(err, errNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read cluster ID=%s, got error: %s", data.Id.ValueString(), err),
		)
		return
	}

	ipWhitelist := []ipWhitelistModel{}

	for _, item := range allowlist {
		ipDesc := ipWhitelistModel{
			IP:          types.StringValue(item.Ip),
			Description: types.StringValue(item.Description),
		}
		ipWhitelist = append(ipWhitelist, ipDesc)
	}

	data.IPWhitelist = ipWhitelist
	// An import only sets the id, which is the cluster ID.
	data.ClusterID = data.Id

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *CamundaClusterIPWhiteListResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data camundaClusterIPWhitelistData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	clusterId := data.ClusterID.ValueString()
	ipWhitelistPath := path.Root("ip_whitelist")

	err := r.client.SetIPAllowlist(ctx, clusterId, ipAllowlistFromState(data))
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			ipWhitelistPath,
			"Unable to configure IP whitelisting",
			err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaClusterIPWhiteListResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data camundaClusterIPWhitelistData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Clearing the allowlist removes all IP restrictions from the cluster.
	err := r.client.SetIPAllowlist(ctx, data.ClusterID.ValueString(), nil)
	if ignoreNotFound(err) != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to remove IP whitelisting from cluster ID=%s, got error: %s", data.Id.ValueString(), err),
		)
		return
	}
}

func (r *CamundaClusterIPWhiteListResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
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
