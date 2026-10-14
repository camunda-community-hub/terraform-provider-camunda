package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestClusterClientDataSources(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: clusterConfig(f, "cluster", fakeTrialPlan, false) + `
resource "camunda_cluster_client" "worker" {
  cluster_id = camunda_cluster.test.id
  name       = "worker"
  scopes     = ["Zeebe", "Operate"]
}

data "camunda_cluster_client" "one" {
  cluster_id      = camunda_cluster.test.id
  zeebe_client_id = camunda_cluster_client.worker.zeebe_client_id
}

data "camunda_cluster_clients" "all" {
  cluster_id = camunda_cluster.test.id
  depends_on = [camunda_cluster_client.worker]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.camunda_cluster_client.one", "id", "camunda_cluster_client.worker", "id"),
					resource.TestCheckResourceAttr("data.camunda_cluster_client.one", "name", "worker"),
					resource.TestCheckResourceAttr("data.camunda_cluster_client.one", "scopes.#", "2"),
					resource.TestCheckResourceAttr("data.camunda_cluster_client.one", "zeebe_address", fakeZeebeAddress),
					resource.TestCheckResourceAttr("data.camunda_cluster_clients.all", "clients.#", "1"),
					resource.TestCheckResourceAttr("data.camunda_cluster_clients.all", "clients.0.name", "worker"),
					resource.TestCheckResourceAttr("data.camunda_cluster_clients.all", "clients.0.zeebe_address", fakeZeebeAddress),
					resource.TestCheckResourceAttrPair("data.camunda_cluster_clients.all", "clients.0.zeebe_client_id", "camunda_cluster_client.worker", "zeebe_client_id"),
				),
			},
		},
	})
}

func TestClusterClientDataSourceReportsUnknownClient(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: clusterConfig(f, "cluster", fakeTrialPlan, false) + `
data "camunda_cluster_client" "missing" {
  cluster_id      = camunda_cluster.test.id
  zeebe_client_id = "nope"
}
`,
				ExpectError: regexp.MustCompile(`nope`),
			},
		},
	})
}

func TestClusterConnectorSecretsDataSource(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: clusterConfig(f, "cluster", fakeTrialPlan, false) + `
resource "camunda_cluster_connector_secret" "b" {
  cluster_id = camunda_cluster.test.id
  name       = "B_KEY"
  value      = "b"
}

resource "camunda_cluster_connector_secret" "a" {
  cluster_id = camunda_cluster.test.id
  name       = "A_KEY"
  value      = "a"
}

data "camunda_cluster_connector_secrets" "all" {
  cluster_id = camunda_cluster.test.id
  depends_on = [camunda_cluster_connector_secret.a, camunda_cluster_connector_secret.b]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.camunda_cluster_connector_secrets.all", "names.#", "2"),
					resource.TestCheckResourceAttr("data.camunda_cluster_connector_secrets.all", "names.0", "A_KEY"),
					resource.TestCheckResourceAttr("data.camunda_cluster_connector_secrets.all", "names.1", "B_KEY"),
				),
			},
		},
	})
}
