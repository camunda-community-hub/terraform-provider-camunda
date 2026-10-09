package provider

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ provider.Provider = &CamundaCloudProvider{}

// CamundaCloudProvider satisfies the provider.Provider interface. Its Configure
// hands a *consoleClient to every resource and data source.
type CamundaCloudProvider struct {
	// tuneClient, when set, adjusts the Console client after it is built.
	// Tests use it to shorten cluster health polling.
	tuneClient func(*consoleClient)
}

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
				MarkdownDescription: "Client ID of an Administration API client. Can also be set with the `CAMUNDA_CONSOLE_CLIENT_ID` environment variable.",
				Optional:            true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "Client secret of an Administration API client. Can also be set with the `CAMUNDA_CONSOLE_CLIENT_SECRET` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"debug": schema.BoolAttribute{
				MarkdownDescription: "Enable debug logs",
				Optional:            true,
			},
			"api_url": schema.StringAttribute{
				MarkdownDescription: "URL of the Camunda SaaS Administration API. Can also be set with the `CAMUNDA_CONSOLE_BASE_URL` environment variable. Defaults to `https://api.cloud.camunda.io`.",
				Optional:            true,
			},
			"token_url": schema.StringAttribute{
				MarkdownDescription: "URL to fetch the OAuth token from. Can also be set with the `CAMUNDA_OAUTH_URL` environment variable. Defaults to `https://login.cloud.camunda.io/oauth/token`.",
				Optional:            true,
			},
			"audience": schema.StringAttribute{
				MarkdownDescription: "Audience of the token. Can also be set with the `CAMUNDA_CONSOLE_OAUTH_AUDIENCE` environment variable. Defaults to the host of `api_url`.",
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

	for name, value := range map[string]types.String{
		"client_id":     data.ClientID,
		"client_secret": data.ClientSecret,
		"api_url":       data.ApiUrl,
		"token_url":     data.TokenUrl,
		"audience":      data.Audience,
	} {
		if value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root(name),
				"Unknown Provider Configuration",
				fmt.Sprintf("The provider can't connect to Camunda SaaS while %s is unknown. Set it to a value known at plan time.", name),
			)
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	clientID := configOrEnv(data.ClientID, "CAMUNDA_CONSOLE_CLIENT_ID", "")
	clientSecret := configOrEnv(data.ClientSecret, "CAMUNDA_CONSOLE_CLIENT_SECRET", "")
	apiURL := configOrEnv(data.ApiUrl, "CAMUNDA_CONSOLE_BASE_URL", "https://api.cloud.camunda.io")
	tokenURL := configOrEnv(data.TokenUrl, "CAMUNDA_OAUTH_URL", "https://login.cloud.camunda.io/oauth/token")

	if clientID == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("client_id"),
			"Missing Client ID",
			"Set client_id in the provider configuration or the CAMUNDA_CONSOLE_CLIENT_ID environment variable.",
		)
	}
	if clientSecret == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("client_secret"),
			"Missing Client Secret",
			"Set client_secret in the provider configuration or the CAMUNDA_CONSOLE_CLIENT_SECRET environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	parsedAPIURL, err := url.Parse(apiURL)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Provider Error",
			fmt.Sprintf("Unable to parse API URL: %v", err),
		)
		return
	}

	audience := configOrEnv(data.Audience, "CAMUNDA_CONSOLE_OAUTH_AUDIENCE", parsedAPIURL.Host)

	client, err := newConsoleClient(ctx, consoleClientConfig{
		APIURL:       apiURL,
		TokenURL:     tokenURL,
		Audience:     audience,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Debug:        data.Debug.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Provider Error", err.Error())
		return
	}

	if p.tuneClient != nil {
		p.tuneClient(client)
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

// configOrEnv returns the configured value, else the environment variable,
// else the fallback.
func configOrEnv(value types.String, envVar, fallback string) string {
	if !value.IsNull() {
		return value.ValueString()
	}
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return fallback
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
