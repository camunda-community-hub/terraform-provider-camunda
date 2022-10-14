package provider

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	console "github.com/sijoma/console-customer-api-go"
	"golang.org/x/oauth2/clientcredentials"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ provider.Provider = &CamundaCloudProvider{}

// CamundaCloudProvider satisfies the CamundaCloudProvider.Provider interface and usually is included
// with all Resource and DataSource implementations.
type CamundaCloudProvider struct {
	client      *console.APIClient
	accessToken string
}

// providerData can be used to store data from the Terraform configuration.
type providerData struct {
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
	Debug        types.Bool   `tfsdk:"debug"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CamundaCloudProvider{}
	}
}

func (p CamundaCloudProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "camunda"
}

func (p *CamundaCloudProvider) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		Attributes: map[string]tfsdk.Attribute{
			"client_id": {
				MarkdownDescription: "Client ID to authenticate against Camunda SaaS",
				Required:            true,
				Type:                types.StringType,
			},
			"client_secret": {
				MarkdownDescription: "Client Secret to authenticate against Camunda SaaS",
				Required:            true,
				Type:                types.StringType,
			},
			"debug": {
				MarkdownDescription: "Enable debug logs",
				Required:            false,
				Optional:            true,
				Type:                types.BoolType,
			},
		},
	}, nil
}

func (p *CamundaCloudProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerData
	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	config := clientcredentials.Config{
		ClientID:     data.ClientID.Value,
		ClientSecret: data.ClientSecret.Value,
		TokenURL:     "https://login.cloud.camunda.io/oauth/token",
		EndpointParams: url.Values{
			"audience": []string{"api.cloud.camunda.io"},
		},
	}

	token, err := config.Token(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Provider Error",
			fmt.Sprintf("Unable to get token: %v", err),
		)
		return
	}

	p.accessToken = token.AccessToken

	cfg := console.NewConfiguration()
	cfg.Scheme = "https"
	cfg.Host = "api.cloud.camunda.io"
	cfg.Debug = data.Debug.Value
	client := console.NewAPIClient(cfg)
	p.client = client

	resp.DataSourceData = p
	resp.ResourceData = p
}

func (p *CamundaCloudProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewCamundaClusterResource,
		NewCamundaClusterClientResource,
	}
}

func (p *CamundaCloudProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewCamundaChannelDataSource,
	}
}
