package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/multani/terraform-provider-camunda/internal/validators"
	console "github.com/sijoma/console-customer-api-go"
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
	provider *CamundaCloudProvider
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

	provider, ok := req.ProviderData.(*CamundaCloudProvider)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *incidentio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.provider = provider
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
	err := r.configureIPWhitelisting(ctx, data, clusterId)
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

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	cluster, response, err := r.provider.client.ClustersApi.GetCluster(ctx, data.Id.ValueString()).Execute()
	if err != nil && response.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read cluster ID=%s, got error: %s", data.Id.ValueString(), err.(*console.GenericOpenAPIError).Body()),
		)
		return
	}

	ipWhitelist := []ipWhitelistModel{}

	for _, item := range cluster.Ipwhitelist {
		ipDesc := ipWhitelistModel{
			IP:          types.StringValue(item.Ip),
			Description: types.StringValue(item.Description),
		}
		ipWhitelist = append(ipWhitelist, ipDesc)
	}

	data.IPWhitelist = ipWhitelist

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

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

	clusterId := data.Id.ValueString()
	ipWhitelistPath := path.Root("ip_whitelist")

	err := r.configureIPWhitelisting(ctx, data, clusterId)
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

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)
	clusterId := data.ClusterID.ValueString()

	err := r.configureIPWhitelisting(ctx, data, clusterId)
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to remove IP whitelisting from cluster ID=%s, got error: %s", data.Id.ValueString(), err.(console.GenericOpenAPIError).Body()),
		)
		return
	}
}

func (r *CamundaClusterIPWhiteListResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *CamundaClusterIPWhiteListResource) configureIPWhitelisting(ctx context.Context, data camundaClusterIPWhitelistData, clusterID string) error {
	ipWhitelist := []console.ClusterIpwhitelistInner{}
	for _, item := range data.IPWhitelist {
		ipWhitelist = append(ipWhitelist, *console.NewClusterIpwhitelistInner(
			item.Description.ValueString(),
			item.IP.ValueString(),
		))
	}

	newIPWhitelistBody := console.IpWhiteListBody{
		Ipwhitelist: ipWhitelist,
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	response, err := r.provider.client.
		ClustersApi.
		UpdateIpWhitelist(ctx, clusterID).
		IpWhiteListBody(newIPWhitelistBody).
		Execute()

	if err != nil {
		return fmt.Errorf("Unable to create cluster, got error: %s", err.(*console.GenericOpenAPIError).Body())
	}

	if response.StatusCode != 204 {
		return fmt.Errorf("Error while configuring IP whitelisting, expected HTTP 200, got: %d", response.StatusCode)
	}

	tflog.Info(ctx, "IP Whitelisting configured", map[string]interface{}{
		"clusterID": data.Id,
	})

	return nil
}
