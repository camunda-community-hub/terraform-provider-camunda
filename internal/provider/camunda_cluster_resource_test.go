package provider

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestClusterResourceLifecycle(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	var clusterID string

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		CheckDestroy: func(*terraform.State) error {
			return noneLeft(state, func(s *consoleState) int { return len(s.clusters) }, "clusters")
		},
		Steps: []resource.TestStep{
			{
				Config: clusterConfig(f, "one", fakeTrialPlan, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("camunda_cluster.test", "id", func(id string) error {
						clusterID = id
						return nil
					}),
					resource.TestCheckResourceAttr("camunda_cluster.test", "name", "one"),
					resource.TestCheckResourceAttr("camunda_cluster.test", "channel", fakeChannelID),
					resource.TestCheckResourceAttr("camunda_cluster.test", "region", fakeRegionID),
					resource.TestCheckResourceAttr("camunda_cluster.test", "plan_type", fakeTrialPlanID),
					resource.TestCheckResourceAttr("camunda_cluster.test", "generation", fakeGeneration1),
					resource.TestCheckResourceAttr("camunda_cluster.test", "current_generation", fakeGeneration1),
				),
			},
			{
				ResourceName:      "camunda_cluster.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: clusterConfig(f, "two", fakeTrialPlan, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("camunda_cluster.test", "id", func(id string) error {
						if id != clusterID {
							return fmt.Errorf("cluster was replaced: %s -> %s", clusterID, id)
						}
						return nil
					}),
					resource.TestCheckResourceAttr("camunda_cluster.test", "name", "two"),
					func(*terraform.State) error {
						var name string
						state.do(func(s *consoleState) { name = s.clusters[clusterID].Name })
						if name != "two" {
							return fmt.Errorf("cluster not renamed in the Console, name = %q", name)
						}
						return nil
					},
				),
			},
			{
				// The plan type can't change in place, so the cluster is replaced.
				Config: clusterConfig(f, "two", fakeProdPlan, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster.test", "plan_type", fakeProdPlanID),
					resource.TestCheckResourceAttrWith("camunda_cluster.test", "id", func(id string) error {
						if id == clusterID {
							return fmt.Errorf("expected the cluster to be replaced, still %s", id)
						}
						return nil
					}),
					func(*terraform.State) error {
						return countIs(state, func(s *consoleState) int { return len(s.clusters) }, 1, "clusters")
					},
				),
			},
		},
	})
}

func TestClusterResourceAutoUpdateGenerationDrift(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	config := clusterConfig(f, "auto", fakeTrialPlan, true)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{Config: config},
			{
				// Camunda upgrades the cluster; the configuration stays valid.
				PreConfig: func() {
					state.do(func(s *consoleState) {
						for _, cluster := range s.clusters {
							cluster.Generation = console.ClusterGeneration{Uuid: fakeGeneration2}
						}
					})
				},
				Config:   config,
				PlanOnly: true,
			},
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster.test", "generation", fakeGeneration1),
					resource.TestCheckResourceAttr("camunda_cluster.test", "current_generation", fakeGeneration2),
				),
			},
		},
	})
}

func TestClusterResourceGenerationChangeWithAutoUpdateKeepsCluster(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)
	withGeneration := func(generation string) string {
		return strings.Replace(clusterConfig(f, "auto", fakeTrialPlan, true),
			"data.camunda_channel.stable.default_generation_id", strconv.Quote(generation), 1)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			// State holding a newer generation than the configuration, as
			// earlier provider versions stored after a Camunda upgrade.
			{Config: withGeneration(fakeGeneration2)},
			{
				Config: withGeneration(fakeGeneration1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("camunda_cluster.test", plancheck.ResourceActionUpdate),
					},
				},
			},
		},
	})
}

func TestClusterResourceDeletedOutsideTerraform(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	config := clusterConfig(f, "gone", fakeTrialPlan, false)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() {
					state.do(func(s *consoleState) { clear(s.clusters) })
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// noneLeft fails when the Console still holds objects after destroy.
func noneLeft(state *consoleState, count func(*consoleState) int, what string) error {
	return countIs(state, count, 0, what)
}

// countIs fails unless the Console holds exactly want objects.
func countIs(state *consoleState, count func(*consoleState) int, want int, what string) error {
	var n int
	state.do(func(s *consoleState) { n = count(s) })
	if n != want {
		return fmt.Errorf("Console holds %d %s, want %d", n, what, want)
	}
	return nil
}
