package provider

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ provider.Provider = &CamundaCloudProvider{}

// CamundaCloudProvider satisfies the provider.Provider interface. Its Configure
// hands a *consoleClient to every resource and data source.
type CamundaCloudProvider struct{}

// providerData can be used to store data from the Terraform configuration.
type providerData struct {
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
	ApiUrl       types.String `tfsdk:"api_url"`
	TokenUrl     types.String `tfsdk:"token_url"`
	Audience     types.String `tfsdk:"audience"`
	Debug        types.Bool   `tfsdk:"debug"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CamundaCloudProvider{}
	}
}

func (p *CamundaCloudProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "camunda"
}

func (p *CamundaCloudProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"client_id": schema.StringAttribute{
				MarkdownDescription: "Client ID to authenticate against Camunda SaaS",
				Required:            true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "Client Secret to authenticate against Camunda SaaS",
				Required:            true,
			},
			"debug": schema.BoolAttribute{
				MarkdownDescription: "Enable debug logs",
				Required:            false,
				Optional:            true,
			},
			"api_url": schema.StringAttribute{
				MarkdownDescription: "URL to Camunda SaaS API",
				Required:            false,
				Optional:            true,
			},
			"token_url": schema.StringAttribute{
				MarkdownDescription: "URL to fetch token from",
				Required:            false,
				Optional:            true,
			},
			"audience": schema.StringAttribute{
				MarkdownDescription: "Audience of the token",
				Required:            false,
				Optional:            true,
			},
		},
	}
}

func (p *CamundaCloudProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerData
	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiURL := "https://api.cloud.camunda.io"
	if !data.ApiUrl.IsNull() {
		apiURL = data.ApiUrl.ValueString()
	}

	tokenURL := "https://login.cloud.camunda.io/oauth/token"
	if !data.TokenUrl.IsNull() {
		tokenURL = data.TokenUrl.ValueString()
	}

	parsedAPIURL, err := url.Parse(apiURL)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Provider Error",
			fmt.Sprintf("Unable to parse API URL: %v", err),
		)
		return
	}

	audience := parsedAPIURL.Host
	if !data.Audience.IsNull() {
		audience = data.Audience.ValueString()
	}

	client, err := newConsoleClient(ctx, consoleClientConfig{
		APIURL:       apiURL,
		TokenURL:     tokenURL,
		Audience:     audience,
		ClientID:     data.ClientID.ValueString(),
		ClientSecret: data.ClientSecret.ValueString(),
		Debug:        data.Debug.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Provider Error", err.Error())
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *CamundaCloudProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewCamundaClusterClientResource,
		NewCamundaClusterConnectorSecretResource,
		NewCamundaClusterIPWhitelistResource,
		NewCamundaClusterResource,
		NewCamundaOrganizationMemberResource,
	}
}

func (p *CamundaCloudProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewCamundaChannelDataSource,
		NewCamundaClusterPlanTypeDataSource,
		NewCamundaRegionDataSource,
	}
}
