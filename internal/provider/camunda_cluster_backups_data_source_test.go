package provider

import (
	"testing"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestClusterBackupsDataSource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: clusterConfig(f, "cluster", fakeTrialPlan, false) + `
data "camunda_cluster_backups" "all" {
  cluster_id = camunda_cluster.test.id
}
`,
				Check: resource.TestCheckResourceAttr("data.camunda_cluster_backups.all", "backups.#", "0"),
			},
			{
				PreConfig: func() {
					state.do(func(s *consoleState) {
						for clusterID := range s.clusters {
							generation, name := "gen-1", "8.7"
							s.backups[clusterID] = []console.BackupDto{{
								Uuid: "backup-1", Name: "nightly", Created: "2026-01-01T00:00:00Z", Completed: "2026-01-01T00:10:00Z",
								Status: console.BACKUPSTATUS_COMPLETE, ZeebeStatus: console.BACKUPSTATUS_COMPLETE,
								OperateStatus: console.BACKUPSTATUS_COMPLETE, TasklistStatus: console.BACKUPSTATUS_COMPLETE,
								OptimizeStatus: console.BACKUPSTATUS_MINUS, GenerationUuid: &generation, GenerationName: &name,
							}}
						}
					})
				},
				Config: clusterConfig(f, "cluster", fakeTrialPlan, false) + `
data "camunda_cluster_backups" "all" {
  cluster_id = camunda_cluster.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.camunda_cluster_backups.all", "backups.#", "1"),
					resource.TestCheckResourceAttr("data.camunda_cluster_backups.all", "backups.0.id", "backup-1"),
					resource.TestCheckResourceAttr("data.camunda_cluster_backups.all", "backups.0.name", "nightly"),
					resource.TestCheckResourceAttr("data.camunda_cluster_backups.all", "backups.0.status", "Complete"),
					resource.TestCheckResourceAttr("data.camunda_cluster_backups.all", "backups.0.optimize_status", "-"),
					resource.TestCheckResourceAttr("data.camunda_cluster_backups.all", "backups.0.generation_name", "8.7"),
				),
			},
		},
	})
}
