package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func clusterClientConfig(f *fakeConsole, name string) string {
	return clusterConfig(f, "cluster", fakeTrialPlan, false) + fmt.Sprintf(`
resource "camunda_cluster_client" "test" {
  cluster_id = camunda_cluster.test.id
  name       = %q
  scopes     = ["Zeebe"]
}
`, name)
}

func TestClusterClientResource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	clientNames := func() []string {
		var names []string
		state.do(func(s *consoleState) {
			for _, clients := range s.clients {
				for _, client := range clients {
					names = append(names, client.Name)
				}
			}
		})
		return names
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		CheckDestroy: func(*terraform.State) error {
			if names := clientNames(); len(names) != 0 {
				return fmt.Errorf("clients left in the Console after destroy: %v", names)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: clusterClientConfig(f, "worker"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("camunda_cluster_client.test", "cluster_id", "camunda_cluster.test", "id"),
					resource.TestCheckResourceAttr("camunda_cluster_client.test", "name", "worker"),
					resource.TestCheckResourceAttr("camunda_cluster_client.test", "scopes.#", "1"),
					resource.TestCheckResourceAttrSet("camunda_cluster_client.test", "secret"),
					resource.TestCheckResourceAttrSet("camunda_cluster_client.test", "zeebe_client_id"),
					resource.TestCheckResourceAttr("camunda_cluster_client.test", "zeebe_address", fakeZeebeAddress),
				),
			},
			{
				// Renaming replaces the client.
				Config: clusterClientConfig(f, "renamed"),
				Check: func(*terraform.State) error {
					if names := clientNames(); len(names) != 1 || names[0] != "renamed" {
						return fmt.Errorf("expected only the renamed client in the Console, got %v", names)
					}
					return nil
				},
			},
		},
	})
}
