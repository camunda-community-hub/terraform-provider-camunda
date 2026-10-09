package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// channelLookup is a data source that makes Terraform configure the provider.
var channelLookup = fmt.Sprintf(`
data "camunda_channel" "test" {
  name = %q
}
`, fakeChannelName)

func TestProviderReadsSettingsFromEnvironment(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)
	t.Setenv("CAMUNDA_CONSOLE_CLIENT_ID", "id")
	t.Setenv("CAMUNDA_CONSOLE_CLIENT_SECRET", "secret")
	t.Setenv("CAMUNDA_CONSOLE_BASE_URL", f.URL)
	t.Setenv("CAMUNDA_OAUTH_URL", f.URL+"/oauth/token")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{{
			Config: `provider "camunda" {}` + channelLookup,
			Check:  resource.TestCheckResourceAttr("data.camunda_channel.test", "id", fakeChannelID),
		}},
	})
}

func TestProviderConfigOverridesEnvironment(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)
	t.Setenv("CAMUNDA_CONSOLE_CLIENT_SECRET", "wrong")
	t.Setenv("CAMUNDA_CONSOLE_BASE_URL", "http://127.0.0.1:1")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{{
			Config: f.providerConfig() + channelLookup,
			Check:  resource.TestCheckResourceAttr("data.camunda_channel.test", "id", fakeChannelID),
		}},
	})
}

func TestProviderRequiresCredentials(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)
	t.Setenv("CAMUNDA_CONSOLE_CLIENT_ID", "")
	t.Setenv("CAMUNDA_CONSOLE_CLIENT_SECRET", "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{{
			Config:      fmt.Sprintf(`provider "camunda" { api_url = %q }`, f.URL) + channelLookup,
			ExpectError: regexp.MustCompile(`CAMUNDA_CONSOLE_CLIENT_SECRET`),
		}},
	})
}
