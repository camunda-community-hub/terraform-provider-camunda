package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestParameterDataSources(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: f.providerConfig() + fmt.Sprintf(`
data "camunda_channel" "test" {
  name = %q
}

data "camunda_region" "test" {
  name = %q
}

data "camunda_cluster_plan_type" "test" {
  name = %q
}
`, fakeChannelName, fakeRegionName, fakeProdPlan),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.camunda_channel.test", "id", fakeChannelID),
					resource.TestCheckResourceAttr("data.camunda_channel.test", "default_generation_id", fakeGeneration1),
					resource.TestCheckResourceAttr("data.camunda_channel.test", "allowed_generations.#", "2"),
					resource.TestCheckResourceAttr("data.camunda_channel.test", "allowed_generations.1.id", fakeGeneration2),
					resource.TestCheckResourceAttr("data.camunda_channel.test", "allowed_generation_ids.%", "2"),
					resource.TestCheckResourceAttr("data.camunda_channel.test", "allowed_generation_ids.8.7", fakeGeneration1),
					resource.TestCheckResourceAttr("data.camunda_channel.test", "allowed_generation_ids.8.8", fakeGeneration2),
					resource.TestCheckResourceAttr("data.camunda_region.test", "id", fakeRegionID),
					resource.TestCheckResourceAttr("data.camunda_cluster_plan_type.test", "id", fakeProdPlanID),
				),
			},
		},
	})
}

func TestParameterDataSourcesReportUnknownNames(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	for dataSource, kind := range map[string]string{
		"camunda_channel":           "channel",
		"camunda_region":            "region",
		"camunda_cluster_plan_type": "clusterPlanType",
	} {
		t.Run(dataSource, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: f.providerFactories(),
				Steps: []resource.TestStep{{
					Config: f.providerConfig() + fmt.Sprintf(`
data %q "test" {
  name = "does-not-exist"
}
`, dataSource),
					ExpectError: regexp.MustCompile(fmt.Sprintf(`%s 'does-not-exist' not found`, kind)),
				}},
			})
		})
	}
}
