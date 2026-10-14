package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func monitoringConfig(f *fakeConsole) string {
	return clusterConfig(f, "cluster", fakeTrialPlan, false) + `
resource "camunda_cluster_monitoring" "test" {
  cluster_id = camunda_cluster.test.id
}
`
}

func TestClusterMonitoringResource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	activeClusters := func() int {
		var n int
		state.do(func(s *consoleState) { n = len(s.monitoringPolls) })
		return n
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		CheckDestroy: func(*terraform.State) error {
			if n := activeClusters(); n != 0 {
				return fmt.Errorf("external monitoring left activated after destroy: %d", n)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: monitoringConfig(f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("camunda_cluster_monitoring.test", "id", "camunda_cluster.test", "id"),
					// The resource only finishes creating once the metrics endpoint exists.
					resource.TestCheckResourceAttr("camunda_cluster_monitoring.test", "metrics_target", "metrics.example.com"),
					resource.TestCheckResourceAttr("camunda_cluster_monitoring.test", "metrics_scheme", "https"),
					resource.TestCheckResourceAttr("camunda_cluster_monitoring.test", "metrics_path", "/metrics"),
				),
			},
			{
				ResourceName:      "camunda_cluster_monitoring.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Deactivated outside Terraform: activated again.
				PreConfig: func() {
					state.do(func(s *consoleState) { clear(s.monitoringPolls) })
				},
				Config: monitoringConfig(f),
				Check: func(*terraform.State) error {
					if n := activeClusters(); n != 1 {
						return fmt.Errorf("expected external monitoring to be activated again, got %d", n)
					}
					return nil
				},
			},
		},
	})
}

func monitoringClientConfig(f *fakeConsole, trigger string) string {
	return monitoringConfig(f) + fmt.Sprintf(`
resource "camunda_cluster_monitoring_client" "test" {
  cluster_id       = camunda_cluster.test.id
  username         = "scraper"
  rotation_trigger = %q

  depends_on = [camunda_cluster_monitoring.test]
}
`, trigger)
}

func TestClusterMonitoringClientResource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	var clientID, password string
	remember := func(dst *string) func(string) error {
		return func(value string) error {
			*dst = value
			return nil
		}
	}
	clientCount := func() int {
		var n int
		state.do(func(s *consoleState) {
			for _, clients := range s.monitoringClients {
				n += len(clients)
			}
		})
		return n
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		CheckDestroy: func(*terraform.State) error {
			if n := clientCount(); n != 0 {
				return fmt.Errorf("%d monitoring clients left after destroy", n)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: monitoringClientConfig(f, "v1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster_monitoring_client.test", "username", "scraper"),
					resource.TestCheckResourceAttrWith("camunda_cluster_monitoring_client.test", "client_id", remember(&clientID)),
					resource.TestCheckResourceAttrWith("camunda_cluster_monitoring_client.test", "password", remember(&password)),
					resource.TestCheckResourceAttr("camunda_cluster_monitoring_client.test", "created", "2026-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("camunda_cluster_monitoring_client.test", "created_by_name", "User One"),
				),
			},
			{
				ResourceName:      "camunda_cluster_monitoring_client.test",
				ImportState:       true,
				ImportStateVerify: true,
				// The API only returns the password when creating the client or rotating it.
				ImportStateVerifyIgnore: []string{"password", "rotation_trigger"},
			},
			{
				// Changing the trigger rotates the password of the same client.
				Config: monitoringClientConfig(f, "v2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("camunda_cluster_monitoring_client.test", "client_id", func(id string) error {
						if id != clientID {
							return fmt.Errorf("client was replaced: %s -> %s", clientID, id)
						}
						return nil
					}),
					resource.TestCheckResourceAttrWith("camunda_cluster_monitoring_client.test", "password", func(rotated string) error {
						if rotated == "" || rotated == password {
							return fmt.Errorf("password was not rotated: %q", rotated)
						}
						return nil
					}),
				),
			},
			{
				// A client deleted outside Terraform is created again.
				PreConfig: func() {
					state.do(func(s *consoleState) { clear(s.monitoringClients) })
				},
				Config: monitoringClientConfig(f, "v2"),
				Check: func(*terraform.State) error {
					if n := clientCount(); n != 1 {
						return fmt.Errorf("expected the client to be created again, got %d", n)
					}
					return nil
				},
			},
		},
	})
}
