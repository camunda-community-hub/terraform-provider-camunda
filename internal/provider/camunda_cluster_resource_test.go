package provider

import (
	"fmt"
	"regexp"
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

func clusterWithSettingsConfig(f *fakeConsole, settings string) string {
	return clusterConfig(f, "cluster", fakeTrialPlan, false) + fmt.Sprintf(`
resource "camunda_cluster" "settings" {
  name       = "settings"
  channel    = data.camunda_channel.stable.id
  region     = data.camunda_region.belgium.id
  plan_type  = data.camunda_cluster_plan_type.plan.id
  generation = data.camunda_channel.stable.default_generation_id
  %s
}
`, settings)
}

func TestClusterResourceSettings(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	var clusterID string
	expectSettings := func(stage console.CamundaClusterStage, packages float64) resource.TestCheckFunc {
		return func(*terraform.State) error {
			var got clusterSettings
			state.do(func(s *consoleState) { got = *s.clusterSettings[clusterID] })
			if got.stageLabel != stage || got.hardwarePackages != packages {
				return fmt.Errorf("Console settings = %+v, want stage %q and %v packages", got, stage, packages)
			}
			return nil
		}
	}
	keepsCluster := func(*terraform.State) error {
		var found bool
		state.do(func(s *consoleState) { _, found = s.clusters[clusterID] })
		if !found {
			return fmt.Errorf("cluster %s was replaced", clusterID)
		}
		return nil
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: clusterWithSettingsConfig(f, `
  stage_label       = "dev"
  hardware_packages = 2
  backup_region     = data.camunda_region.belgium.id
  encryption        = "Software"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("camunda_cluster.settings", "id", func(id string) error {
						clusterID = id
						return nil
					}),
					expectSettings(console.CAMUNDACLUSTERSTAGE_DEV, 2),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "backup_region", fakeRegionID),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "encryption", "Software"),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "owner_id", "owner-1"),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "status", "Healthy"),
					resource.TestCheckResourceAttrSet("camunda_cluster.settings", "created"),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "labels.#", "0"),
					resource.TestMatchResourceAttr("camunda_cluster.settings", "links.zeebe", regexp.MustCompile(`^https://cluster-\d+\.zeebe\.example\.com$`)),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "links.operate", ""),
				),
			},
			{
				// Stage label and hardware packages change in place.
				Config: clusterWithSettingsConfig(f, `
  stage_label       = "prod"
  hardware_packages = 3
  backup_region     = data.camunda_region.belgium.id
  encryption        = "Software"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					keepsCluster,
					expectSettings(console.CAMUNDACLUSTERSTAGE_PROD, 3),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "stage_label", "prod"),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "hardware_packages", "3"),
				),
			},
			{
				// The encryption type can't change in place.
				Config: clusterWithSettingsConfig(f, `
  stage_label       = "prod"
  hardware_packages = 3
  backup_region     = data.camunda_region.belgium.id
  encryption        = "Hardware"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster.settings", "encryption", "Hardware"),
					resource.TestCheckResourceAttrWith("camunda_cluster.settings", "id", func(id string) error {
						if id == clusterID {
							return fmt.Errorf("expected the cluster to be replaced, still %s", id)
						}
						return nil
					}),
				),
			},
		},
	})
}

func TestClusterResourceReportsConsoleDefaults(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: clusterConfig(f, "cluster", fakeTrialPlan, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster.test", "encryption", "Provider"),
					resource.TestCheckNoResourceAttr("camunda_cluster.test", "backup_region"),
					resource.TestCheckNoResourceAttr("camunda_cluster.test", "stage_label"),
				),
			},
			{
				// A second apply plans nothing: the defaults the Console
				// reported don't show up as drift.
				Config:   clusterConfig(f, "cluster", fakeTrialPlan, false),
				PlanOnly: true,
			},
		},
	})
}

func TestClusterResourceReportsEncryptionKeys(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	const primary, secondary = "arn:aws:kms:eu-west-1:1:key/primary", "arn:aws:kms:eu-central-1:1:key/secondary"
	config := clusterWithSettingsConfig(f, `encryption = "External"`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster.settings", "encryption", "External"),
					resource.TestCheckNoResourceAttr("camunda_cluster.settings", "encryption_status"),
					resource.TestCheckNoResourceAttr("camunda_cluster.settings", "encryption_primary_key_arn"),
					resource.TestCheckNoResourceAttr("camunda_cluster.settings", "encryption_secondary_key_arn"),
				),
			},
			{
				// Keys supplied outside this resource show up on the next refresh.
				PreConfig: func() {
					state.do(func(s *consoleState) {
						for _, cluster := range s.clusters {
							if cluster.Encryption.Type == console.CLUSTERENCRYPTIONKEY_EXTERNAL {
								ready := console.ENCRYPTIONSTATUS_READY
								cluster.Encryption.Status = &ready
								cluster.Encryption.PrimaryKeyArn = &[]string{primary}[0]
								cluster.Encryption.SecondaryKeyArn = &[]string{secondary}[0]
							}
						}
					})
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster.settings", "encryption_status", "Ready"),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "encryption_primary_key_arn", primary),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "encryption_secondary_key_arn", secondary),
				),
			},
		},
	})
}

func TestClusterResourceRejectsEmptyBackupRegion(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				// An empty region would be sent as no region and read back as null.
				Config:      clusterWithSettingsConfig(f, `backup_region = ""`),
				ExpectError: regexp.MustCompile(`(?s)backup_region.*string length must be at least 1`),
			},
		},
	})
}

// A cluster with external encryption keys waits for them instead of becoming
// healthy, so creating it must not wait for a health it can't reach yet.
func TestClusterResourceWithExternalEncryptionWaitsForKeysInsteadOfHealth(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: clusterWithSettingsConfig(f, `encryption = "External"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster.settings", "encryption", "External"),
					resource.TestCheckResourceAttr("camunda_cluster.settings", "status", "WaitingForInput"),
				),
			},
			{
				// Nothing to change while it waits.
				Config:   clusterWithSettingsConfig(f, `encryption = "External"`),
				PlanOnly: true,
			},
		},
	})
}
