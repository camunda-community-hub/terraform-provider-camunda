package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	fakeKeyOne = "arn:aws:kms:eu-west-1:123456789012:key/one"
	fakeKeyTwo = "arn:aws:kms:eu-west-1:123456789012:key/two"
)

func byokClusterConfig(f *fakeConsole, encryption string) string {
	return clusterConfig(f, "cluster", fakeTrialPlan, false) + fmt.Sprintf(`
resource "camunda_cluster" "byok" {
  name        = "byok"
  channel     = data.camunda_channel.stable.id
  region      = data.camunda_region.belgium.id
  plan_type   = data.camunda_cluster_plan_type.plan.id
  generation  = data.camunda_channel.stable.default_generation_id
  encryption  = %q
  auto_update = false
}
`, encryption)
}

func encryptionConfig(f *fakeConsole, encryption, primaryKey string) string {
	return byokClusterConfig(f, encryption) + fmt.Sprintf(`
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
				// Changing the key rotates it in place.
				Config: encryptionConfig(f, "External", fakeKeyTwo),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_cluster_encryption.test", "primary_key_arn", fakeKeyTwo),
					resource.TestCheckResourceAttr("camunda_cluster_encryption.test", "status", "Ready"),
				),
			},
			{
				// Destroying leaves the keys on the cluster; the resource is just gone from state.
				Config: byokClusterConfig(f, "External"),
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
