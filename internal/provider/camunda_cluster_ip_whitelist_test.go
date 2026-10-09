package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func ipWhitelistConfig(f *fakeConsole, ips ...string) string {
	var blocks strings.Builder
	for _, ip := range ips {
		fmt.Fprintf(&blocks, `
  ip_whitelist {
    ip          = %q
    description = "office"
  }
`, ip)
	}

	return clusterConfig(f, "cluster", fakeTrialPlan, false) + fmt.Sprintf(`
resource "camunda_cluster_ip_whitelist" "test" {
  cluster_id = camunda_cluster.test.id
%s}
`, blocks.String())
}

func TestClusterIPWhitelistResource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	expectAllowlist := func(want ...string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			var got []string
			state.do(func(s *consoleState) {
				for _, cluster := range s.clusters {
					for _, entry := range cluster.Ipallowlist {
						got = append(got, entry.Ip)
					}
				}
			})
			if fmt.Sprint(got) != fmt.Sprint(want) {
				return fmt.Errorf("Console allowlist = %v, want %v", got, want)
			}
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: ipWhitelistConfig(f, "10.0.0.1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster_ip_whitelist.test", "ip_whitelist.#", "1"),
					expectAllowlist("10.0.0.1"),
				),
			},
			{
				Config: ipWhitelistConfig(f, "10.0.0.1", "172.42.0.0/24"),
				Check:  expectAllowlist("10.0.0.1", "172.42.0.0/24"),
			},
			{
				ResourceName:      "camunda_cluster_ip_whitelist.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Removing the resource lifts the restriction from the cluster.
				Config: clusterConfig(f, "cluster", fakeTrialPlan, false),
				Check:  expectAllowlist(),
			},
		},
	})
}
