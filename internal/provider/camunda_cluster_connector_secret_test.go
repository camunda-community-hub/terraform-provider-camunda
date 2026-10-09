package provider

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func connectorSecretConfig(f *fakeConsole, name, value string) string {
	return clusterConfig(f, "cluster", fakeTrialPlan, false) + fmt.Sprintf(`
resource "camunda_cluster_connector_secret" "test" {
  cluster_id = camunda_cluster.test.id
  name       = %q
  value      = %q
}
`, name, value)
}

func TestClusterConnectorSecretResource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	secrets := func() map[string]string {
		all := map[string]string{}
		state.do(func(s *consoleState) {
			for _, clusterSecrets := range s.secrets {
				maps.Copy(all, clusterSecrets)
			}
		})
		return all
	}
	expectSecrets := func(want map[string]string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			if got := secrets(); !maps.Equal(got, want) {
				return fmt.Errorf("Console secrets = %v, want %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
			}
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: connectorSecretConfig(f, "API_KEY", "one"),
				Check:  expectSecrets(map[string]string{"API_KEY": "one"}),
			},
			{
				// Renaming replaces the secret instead of orphaning the old one.
				Config: connectorSecretConfig(f, "API_TOKEN", "one"),
				Check:  expectSecrets(map[string]string{"API_TOKEN": "one"}),
			},
			{
				Config: connectorSecretConfig(f, "API_TOKEN", "two"),
				Check:  expectSecrets(map[string]string{"API_TOKEN": "two"}),
			},
		},
	})
}
