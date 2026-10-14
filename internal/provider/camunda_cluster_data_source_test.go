package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestClusterDataSources(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: clusterConfig(f, "cluster", fakeTrialPlan, false) + `
data "camunda_cluster" "one" {
  id = camunda_cluster.test.id
}

data "camunda_clusters" "all" {
  depends_on = [camunda_cluster.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.camunda_cluster.one", "name", "cluster"),
					resource.TestCheckResourceAttr("data.camunda_cluster.one", "channel", fakeChannelID),
					resource.TestCheckResourceAttr("data.camunda_cluster.one", "region", fakeRegionID),
					resource.TestCheckResourceAttr("data.camunda_cluster.one", "plan_type", fakeTrialPlanID),
					resource.TestCheckResourceAttr("data.camunda_cluster.one", "generation", fakeGeneration1),
					resource.TestCheckResourceAttr("data.camunda_cluster.one", "auto_update", "false"),
					resource.TestCheckResourceAttr("data.camunda_cluster.one", "encryption", "Provider"),
					resource.TestCheckResourceAttr("data.camunda_cluster.one", "status", "Healthy"),
					resource.TestCheckNoResourceAttr("data.camunda_cluster.one", "description"),
					resource.TestCheckNoResourceAttr("data.camunda_cluster.one", "backup_region"),
					resource.TestMatchResourceAttr("data.camunda_cluster.one", "links.zeebe", regexp.MustCompile(`^https://`)),
					resource.TestCheckResourceAttrPair("data.camunda_cluster.one", "owner_id", "camunda_cluster.test", "owner_id"),
					resource.TestCheckResourceAttr("data.camunda_clusters.all", "clusters.#", "1"),
					resource.TestCheckResourceAttrPair("data.camunda_clusters.all", "clusters.0.id", "camunda_cluster.test", "id"),
					resource.TestCheckResourceAttr("data.camunda_clusters.all", "clusters.0.name", "cluster"),
				),
			},
		},
	})
}

func TestClusterDataSourceReportsUnknownCluster(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: f.providerConfig() + `
data "camunda_cluster" "missing" {
  id = "nope"
}
`,
				ExpectError: regexp.MustCompile(`cluster not found`),
			},
		},
	})
}
