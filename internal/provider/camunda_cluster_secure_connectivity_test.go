package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func secureConnectivityConfig(f *fakeConsole, principals string) string {
	return clusterConfig(f, "cluster", fakeTrialPlan, false) + fmt.Sprintf(`
resource "camunda_cluster_secure_connectivity" "test" {
  cluster_id         = camunda_cluster.test.id
  allowed_principals = %s
  allowed_regions    = ["eu-west-1"]
}
`, principals)
}

func TestClusterSecureConnectivityResource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	activated := func() []*fakeSecureConnectivity {
		var all []*fakeSecureConnectivity
		state.do(func(s *consoleState) {
			for _, sc := range s.secureConnectivity {
				all = append(all, sc)
			}
		})
		return all
	}
	var clusterID string

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		CheckDestroy: func(*terraform.State) error {
			if all := activated(); len(all) != 0 {
				return fmt.Errorf("secure connectivity left activated after destroy: %d", len(all))
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: secureConnectivityConfig(f, `["arn:aws:iam::123:root"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("camunda_cluster_secure_connectivity.test", "id", "camunda_cluster.test", "id"),
					resource.TestCheckResourceAttrWith("camunda_cluster.test", "id", func(id string) error {
						clusterID = id
						return nil
					}),
					resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "allowed_principals.#", "1"),
					resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "allowed_regions.#", "1"),
					// The resource only finishes creating once the endpoint exists.
					resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "service_name", "com.amazonaws.vpce.sc"),
					resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "private_dns_name", "sc.internal"),
					resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "connection_count", "3"),
					resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "conditions.0.type", "Ready"),
					resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "conditions.0.last_transition_time", "2026-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "endpoint_connections.#", "1"),
					resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "endpoint_connections.0.state", "available"),
					resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "endpoint_connections.0.owner", "123456789012"),
				),
			},
			{
				ResourceName:      "camunda_cluster_secure_connectivity.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Principals can't change on an active endpoint, so it is activated again.
				Config: secureConnectivityConfig(f, `["arn:aws:iam::123:root", "arn:aws:iam::456:root"]`),
				Check: func(*terraform.State) error {
					var principals []string
					state.do(func(s *consoleState) { principals = s.secureConnectivity[clusterID].principals })
					if len(principals) != 2 {
						return fmt.Errorf("Console principals = %v, want 2", principals)
					}
					return nil
				},
			},
			{
				// Deactivated outside Terraform: activated again.
				PreConfig: func() {
					state.do(func(s *consoleState) { clear(s.secureConnectivity) })
				},
				Config: secureConnectivityConfig(f, `["arn:aws:iam::123:root", "arn:aws:iam::456:root"]`),
				Check: func(*terraform.State) error {
					if n := len(activated()); n != 1 {
						return fmt.Errorf("expected secure connectivity to be activated again, got %d", n)
					}
					return nil
				},
			},
		},
	})
}

func TestClusterSecureConnectivityResourceKeepsUnfinishedActivation(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) { s.slowSecureConnectivity = true })
	config := secureConnectivityConfig(f, `["arn:aws:iam::123:root"]`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				// The endpoint service is still provisioning when the wait gives up.
				Config:      config,
				ExpectError: regexp.MustCompile(`never became ready`),
			},
			{
				// The refresh must not forget the activation, which is still
				// provisioning: the apply would activate again and fail with
				// "already activated". Instead the tainted resource is replaced
				// and waits again.
				Config:      config,
				ExpectError: regexp.MustCompile(`never became ready`),
			},
			{
				PreConfig: func() {
					state.do(func(s *consoleState) {
						s.slowSecureConnectivity = false
						for _, sc := range s.secureConnectivity {
							sc.pollsLeft = 0
						}
					})
				},
				Config: config,
				Check:  resource.TestCheckResourceAttr("camunda_cluster_secure_connectivity.test", "service_name", "com.amazonaws.vpce.sc"),
			},
		},
	})
}
