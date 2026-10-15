package provider

import (
	"fmt"
	console "github.com/camunda-community-hub/console-customer-api-go"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	// Keys in the primary region of the AWS cluster, and one in its backup region.
	fakeKeyOne       = "arn:aws:kms:eu-central-1:123456789012:key/one"
	fakeKeyTwo       = "arn:aws:kms:eu-central-1:123456789012:key/two"
	fakeKeyBackupOne = "arn:aws:kms:eu-west-1:123456789012:key/backup-one"
)

// byokClusterConfig is a cluster in an AWS region, with a backup region in
// another one if dualRegion.
func byokClusterConfig(f *fakeConsole, encryption string, dualRegion bool) string {
	backupRegion := ""
	if dualRegion {
		backupRegion = "backup_region = data.camunda_region.ireland.id"
	}
	return clusterConfig(f, "cluster", fakeTrialPlan, false) + fmt.Sprintf(`
data "camunda_region" "frankfurt" {
  name = %q
}

data "camunda_region" "ireland" {
  name = %q
}

resource "camunda_cluster" "byok" {
  name        = "byok"
  channel     = data.camunda_channel.stable.id
  region      = data.camunda_region.frankfurt.id
  plan_type   = data.camunda_cluster_plan_type.plan.id
  generation  = data.camunda_channel.stable.default_generation_id
  encryption  = %q
  auto_update = false
  %s
}
`, fakeAWSRegionName, fakeAWSBackupName, encryption, backupRegion)
}

func encryptionConfig(f *fakeConsole, encryption, primaryKey string) string {
	return byokClusterConfig(f, encryption, false) + fmt.Sprintf(`
resource "camunda_cluster_encryption" "test" {
  cluster_id     = camunda_cluster.byok.id
  primary_key_id = %q
}
`, primaryKey)
}

func TestClusterEncryptionResource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: encryptionConfig(f, "External", fakeKeyOne),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("camunda_cluster_encryption.test", "id", "camunda_cluster.byok", "id"),
					// The resource only finishes creating once the encryption is ready.
					resource.TestCheckResourceAttr("camunda_cluster_encryption.test", "status", "Ready"),
					resource.TestCheckResourceAttr("camunda_cluster_encryption.test", "primary_key_arn", fakeKeyOne),
					resource.TestCheckNoResourceAttr("camunda_cluster_encryption.test", "secondary_key_arn"),
				),
			},
			{
				ResourceName:      "camunda_cluster_encryption.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// A key is applied once: changing it is rejected at plan time
				// instead of calling the Console, and the key stays.
				Config:      encryptionConfig(f, "External", fakeKeyTwo),
				ExpectError: regexp.MustCompile(`Encryption\s+key\s+can't\s+be\s+changed`),
			},
			{
				// Destroying leaves the keys on the cluster; the resource is just gone from state.
				Config: byokClusterConfig(f, "External", false),
				Check: func(*terraform.State) error {
					var stillConfigured bool
					state.do(func(s *consoleState) {
						for _, cluster := range s.clusters {
							stillConfigured = stillConfigured || (cluster.Encryption != nil && cluster.Encryption.PrimaryKeyArn != nil)
						}
					})
					if !stillConfigured {
						return fmt.Errorf("the Console has no key configured after destroying the resource, want it left in place")
					}
					return nil
				},
			},
		},
	})
}

func TestClusterEncryptionResourceRejectsClusterWithoutExternalEncryption(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:      encryptionConfig(f, "Software", fakeKeyOne),
				ExpectError: regexp.MustCompile(`not\s+configured\s+for\s+external\s+encryption`),
			},
		},
	})
}

func TestClusterEncryptionResourceDetectsRemovedSecondaryKey(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	config := byokClusterConfig(f, "External", true) + fmt.Sprintf(`
resource "camunda_cluster_encryption" "test" {
  cluster_id       = camunda_cluster.byok.id
  primary_key_id   = %q
  secondary_key_id = %q
}
`, fakeKeyOne, fakeKeyBackupOne)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("camunda_cluster_encryption.test", "secondary_key_id", fakeKeyBackupOne),
			},
			{
				// The Console stops reporting the secondary key: it can't be set again, which the plan reports.
				PreConfig: func() {
					state.do(func(s *consoleState) {
						for _, cluster := range s.clusters {
							if cluster.Encryption != nil {
								cluster.Encryption.SecondaryKeyArn = nil
							}
						}
					})
				},
				Config:      config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Encryption\s+key\s+can't\s+be\s+changed`),
			},
		},
	})
}

func TestClusterEncryptionResourceRejectsEmptyKeyIDs(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	for name, keys := range map[string]string{
		"secondary": fmt.Sprintf(`primary_key_id = %q
  secondary_key_id = ""`, fakeKeyOne),
		"primary": `primary_key_id = ""`,
	} {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: f.providerFactories(),
				Steps: []resource.TestStep{
					{
						// An empty key is sent as no key and read back as null.
						Config: byokClusterConfig(f, "External", false) + `
resource "camunda_cluster_encryption" "test" {
  cluster_id = camunda_cluster.byok.id
  ` + keys + `
}
`,
						ExpectError: regexp.MustCompile(`(?s)_key_id.*string length must be at least 1`),
					},
				},
			})
		})
	}
}

func TestClusterEncryptionResourceEnforcesRegionsAndSecondaryKey(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)
	encryption := func(cluster, keys string) string {
		return cluster + `
resource "camunda_cluster_encryption" "test" {
  cluster_id = camunda_cluster.byok.id
  ` + keys + `
}
`
	}

	tests := map[string]struct {
		config string
		want   string
	}{
		"key in another region": {
			config: encryption(byokClusterConfig(f, "External", false), fmt.Sprintf(`primary_key_id = %q`, fakeKeyBackupOne)),
			want:   `not in the cluster's region`,
		},
		"dual-region without a secondary key": {
			config: encryption(byokClusterConfig(f, "External", true), fmt.Sprintf(`primary_key_id = %q`, fakeKeyOne)),
			want:   `needs a distinct secondary key`,
		},
		"dual-region with a secondary key in the primary region": {
			config: encryption(byokClusterConfig(f, "External", true), fmt.Sprintf("primary_key_id = %q\n  secondary_key_id = %q", fakeKeyOne, fakeKeyTwo)),
			want:   `needs a distinct secondary key`,
		},
		"single-region with a secondary key": {
			config: encryption(byokClusterConfig(f, "External", false), fmt.Sprintf("primary_key_id = %q\n  secondary_key_id = %q", fakeKeyOne, fakeKeyBackupOne)),
			want:   `takes no secondary key`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: f.providerFactories(),
				Steps:                    []resource.TestStep{{Config: tc.config, ExpectError: regexp.MustCompile(strings.ReplaceAll(tc.want, " ", `\s+`))}},
			})
		})
	}
}

func TestClusterEncryptionResourceNeedsAnAWSRegion(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: clusterWithSettingsConfig(f, `encryption = "External"`) + fmt.Sprintf(`
resource "camunda_cluster_encryption" "test" {
  cluster_id     = camunda_cluster.settings.id
  primary_key_id = %q
}
`, fakeKeyOne),
				ExpectError: regexp.MustCompile(`only\s+available\s+on\s+AWS`),
			},
		},
	})
}

func TestClusterEncryptionResourceSupportsDualRegionClusters(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: byokClusterConfig(f, "External", true) + fmt.Sprintf(`
resource "camunda_cluster_encryption" "test" {
  cluster_id       = camunda_cluster.byok.id
  primary_key_id   = %q
  secondary_key_id = %q
}
`, fakeKeyOne, fakeKeyBackupOne),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster_encryption.test", "primary_key_arn", fakeKeyOne),
					resource.TestCheckResourceAttr("camunda_cluster_encryption.test", "secondary_key_arn", fakeKeyBackupOne),
					resource.TestCheckResourceAttr("camunda_cluster_encryption.test", "status", "Ready"),
				),
			},
		},
	})
}

func TestClusterEncryptionResourceRejectsChangingAppliedKeys(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	keyRegistered := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			var got string
			state.do(func(s *consoleState) {
				for _, cluster := range s.clusters {
					if cluster.Encryption != nil && cluster.Encryption.PrimaryKeyArn != nil {
						got = *cluster.Encryption.PrimaryKeyArn
					}
				}
			})
			if got != want {
				return fmt.Errorf("Console primary key = %q, want %q", got, want)
			}
			return nil
		}
	}
	rejected := regexp.MustCompile(`Encryption\s+key\s+can't\s+be\s+changed`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: encryptionConfig(f, "External", fakeKeyOne),
				Check:  keyRegistered(fakeKeyOne),
			},
			{
				Config:      encryptionConfig(f, "External", fakeKeyTwo),
				ExpectError: rejected,
			},
			{
				// Adding a key to applied ones is a change too.
				Config: byokClusterConfig(f, "External", false) + fmt.Sprintf(`
resource "camunda_cluster_encryption" "test" {
  cluster_id       = camunda_cluster.byok.id
  primary_key_id   = %q
  secondary_key_id = %q
}
`, fakeKeyOne, fakeKeyBackupOne),
				ExpectError: rejected,
			},
			{
				// An unchanged configuration plans nothing.
				Config:   encryptionConfig(f, "External", fakeKeyOne),
				PlanOnly: true,
			},
			{
				// And the Console still has the first key.
				Config: encryptionConfig(f, "External", fakeKeyOne),
				Check:  keyRegistered(fakeKeyOne),
			},
		},
	})
}

func TestClusterEncryptionResourceAllowsOtherKeysOnAnotherCluster(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	cluster := func(name string) string {
		return fmt.Sprintf(`
resource "camunda_cluster" %[1]q {
  name        = %[1]q
  channel     = data.camunda_channel.stable.id
  region      = data.camunda_region.frankfurt.id
  plan_type   = data.camunda_cluster_plan_type.plan.id
  generation  = data.camunda_channel.stable.default_generation_id
  encryption  = "External"
  auto_update = false
}
`, name)
	}
	config := func(clusters []string, target, key string) string {
		hcl := byokClusterConfig(f, "External", false)
		for _, name := range clusters {
			hcl += cluster(name)
		}
		return hcl + fmt.Sprintf(`
resource "camunda_cluster_encryption" "other" {
  cluster_id     = camunda_cluster.%s.id
  primary_key_id = %q
}
`, target, key)
	}
	appliedKeys := func() []string {
		var keys []string
		state.do(func(s *consoleState) {
			for _, c := range s.clusters {
				if c.Encryption != nil && c.Encryption.PrimaryKeyArn != nil {
					keys = append(keys, *c.Encryption.PrimaryKeyArn)
				}
			}
		})
		slices.Sort(keys)
		return keys
	}
	expectKeys := func(want ...string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			if got := appliedKeys(); !slices.Equal(got, want) {
				return fmt.Errorf("Console keys = %v, want %v", got, want)
			}
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: config([]string{"first", "second"}, "first", fakeKeyOne),
				Check:  expectKeys(fakeKeyOne),
			},
			{
				// Another cluster that exists: its id is known at plan time and
				// differs, so the other key is fine; the resource is replaced.
				Config: config([]string{"first", "second"}, "second", fakeKeyTwo),
				Check:  expectKeys(fakeKeyOne, fakeKeyTwo),
			},
			{
				// A cluster that doesn't exist yet: its id is unknown at plan time.
				Config: config([]string{"first", "second", "third"}, "third", fakeKeyOne),
				Check:  expectKeys(fakeKeyOne, fakeKeyOne, fakeKeyTwo),
			},
		},
	})
}

func TestClusterEncryptionResourceResumesAfterAFailedWait(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) { s.slowEncryption = true })
	config := encryptionConfig(f, "External", fakeKeyOne)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				// The keys are applied, but the encryption isn't ready when the wait gives up.
				Config:      config,
				ExpectError: regexp.MustCompile(`never became ready`),
			},
			{
				// Reapplying replaces the tainted resource. Destroying it leaves
				// the keys on the cluster, so sending them again would be
				// rejected: it resumes waiting instead.
				PreConfig: func() {
					state.do(func(s *consoleState) {
						s.slowEncryption = false
						for id := range s.encryptionPolls {
							s.encryptionPolls[id] = 0
						}
					})
				},
				Config: config,
				Check:  resource.TestCheckResourceAttr("camunda_cluster_encryption.test", "status", "Ready"),
			},
		},
	})
}

func TestClusterEncryptionResourceRejectsOtherKeysAlreadyApplied(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: byokClusterConfig(f, "External", false),
			},
			{
				// Another key was applied outside Terraform.
				PreConfig: func() {
					state.do(func(s *consoleState) {
						for _, cluster := range s.clusters {
							if cluster.Encryption != nil && cluster.Encryption.Type == console.CLUSTERENCRYPTIONKEY_EXTERNAL {
								key := fakeKeyTwo
								cluster.Encryption.PrimaryKeyArn = &key
							}
						}
					})
				},
				Config:      encryptionConfig(f, "External", fakeKeyOne),
				ExpectError: regexp.MustCompile(`(?s)already\s+has\s+other\s+keys\s+applied`),
			},
		},
	})
}

func TestClusterEncryptionResourceImportNeedsAppliedKeys(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)
	clusterIDOf := func(address string) resource.ImportStateIdFunc {
		return func(s *terraform.State) (string, error) {
			return s.RootModule().Resources[address].Primary.ID, nil
		}
	}
	withResource := func(cluster string) string {
		return cluster + fmt.Sprintf(`
resource "camunda_cluster_encryption" "test" {
  cluster_id     = camunda_cluster.byok.id
  primary_key_id = %q
}
`, fakeKeyOne)
	}
	rejected := regexp.MustCompile(`(?s)no\s+customer-managed\s+keys\s+applied`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				// An External cluster that still waits for its first key.
				Config: byokClusterConfig(f, "External", false),
			},
			{
				Config:            withResource(byokClusterConfig(f, "External", false)),
				ResourceName:      "camunda_cluster_encryption.test",
				ImportState:       true,
				ImportStateIdFunc: clusterIDOf("camunda_cluster.byok"),
				ExpectError:       rejected,
			},
		},
	})
}
