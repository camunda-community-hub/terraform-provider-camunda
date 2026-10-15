package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestIPRangesDataSource(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: f.providerConfig() + `
data "camunda_ip_ranges" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.camunda_ip_ranges.test", "id", "ip-ranges"),
					resource.TestCheckResourceAttr("data.camunda_ip_ranges.test", "web_modeler.#", "2"),
					resource.TestCheckResourceAttr("data.camunda_ip_ranges.test", "web_modeler.0", "10.0.0.0/24"),
					resource.TestCheckResourceAttr("data.camunda_ip_ranges.test", "web_modeler.1", "10.0.1.0/24"),
				),
			},
		},
	})
}
