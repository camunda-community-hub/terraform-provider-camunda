package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func backupScheduleConfig(f *fakeConsole, schedule string) string {
	return clusterConfig(f, "cluster", fakeTrialPlan, false) + fmt.Sprintf(`
resource "camunda_cluster_backup_schedule" "test" {
  cluster_id = camunda_cluster.test.id
  schedule   = %q
}
`, schedule)
}

func TestClusterBackupScheduleResource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	expectSchedules := func(want ...string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			var got []string
			state.do(func(s *consoleState) {
				for _, schedule := range s.schedules {
					got = append(got, schedule)
				}
			})
			if fmt.Sprint(got) != fmt.Sprint(want) {
				return fmt.Errorf("Console schedules = %v, want %v", got, want)
			}
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		CheckDestroy:             func(*terraform.State) error { return expectSchedules()(nil) },
		Steps: []resource.TestStep{
			{
				Config: backupScheduleConfig(f, "0 2 * * *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("camunda_cluster_backup_schedule.test", "id", "camunda_cluster.test", "id"),
					resource.TestCheckResourceAttr("camunda_cluster_backup_schedule.test", "schedule", "0 2 * * *"),
					expectSchedules("0 2 * * *"),
				),
			},
			{
				ResourceName:      "camunda_cluster_backup_schedule.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// The schedule changes in place.
				Config: backupScheduleConfig(f, "30 3 * * 1"),
				Check:  expectSchedules("30 3 * * 1"),
			},
			{
				// A schedule removed outside Terraform is set again.
				PreConfig: func() {
					state.do(func(s *consoleState) { clear(s.schedules) })
				},
				Config: backupScheduleConfig(f, "30 3 * * 1"),
				Check:  expectSchedules("30 3 * * 1"),
			},
			{
				// Removing only the schedule stops scheduled backups while the
				// cluster stays, so the cluster's teardown can't hide a delete
				// that did nothing.
				Config: clusterConfig(f, "cluster", fakeTrialPlan, false),
				Check:  expectSchedules(),
			},
		},
	})
}

func TestClusterBackupScheduleResourceReportsInvalidSchedule(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:      backupScheduleConfig(f, "every day"),
				ExpectError: regexp.MustCompile(`not a valid cron expression`),
			},
		},
	})
}
