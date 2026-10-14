package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func clusterBackupConfig(f *fakeConsole) string {
	return clusterConfig(f, "cluster", fakeTrialPlan, false) + `
resource "camunda_cluster_backup" "test" {
  cluster_id = camunda_cluster.test.id
}
`
}

func countBackups(state *consoleState) int {
	var count int
	state.do(func(s *consoleState) {
		for _, backups := range s.backups {
			count += len(backups)
		}
	})
	return count
}

func TestClusterBackupResource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		CheckDestroy: func(*terraform.State) error {
			if n := countBackups(state); n != 0 {
				return fmt.Errorf("%d backups left in the Console after destroy", n)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: clusterBackupConfig(f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("camunda_cluster_backup.test", "cluster_id", "camunda_cluster.test", "id"),
					resource.TestCheckResourceAttrSet("camunda_cluster_backup.test", "backup_id"),
					resource.TestMatchResourceAttr("camunda_cluster_backup.test", "id", regexp.MustCompile(`^cluster-\d+/backup-\d+$`)),
					// The resource only finishes creating once the backup is complete.
					resource.TestCheckResourceAttr("camunda_cluster_backup.test", "status", "Complete"),
					resource.TestCheckResourceAttr("camunda_cluster_backup.test", "zeebe_status", "Complete"),
					resource.TestCheckResourceAttrSet("camunda_cluster_backup.test", "completed"),
					resource.TestCheckNoResourceAttr("camunda_cluster_backup.test", "generation_id"),
				),
			},
			{
				ResourceName:      "camunda_cluster_backup.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// A backup deleted outside Terraform is created again.
				PreConfig: func() {
					state.do(func(s *consoleState) {
						for id := range s.backups {
							s.backups[id] = nil
						}
					})
				},
				Config: clusterBackupConfig(f),
				Check: func(*terraform.State) error {
					if n := countBackups(state); n != 1 {
						return fmt.Errorf("expected the backup to be created again, %d backups in the Console", n)
					}
					return nil
				},
			},
		},
	})
}

func TestClusterBackupResourceReportsFailedBackup(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) { s.failBackups = true })

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:      clusterBackupConfig(f),
				ExpectError: regexp.MustCompile(`never became ready.*Failed`),
			},
		},
	})
}
