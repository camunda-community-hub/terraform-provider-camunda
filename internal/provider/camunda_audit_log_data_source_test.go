package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAuditLogDataSource(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: f.providerConfig() + `
data "camunda_audit_log" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.camunda_audit_log.test", "events.#", "2"),
					resource.TestCheckResourceAttr("data.camunda_audit_log.test", "events.0.audit", "Cluster created"),
					resource.TestCheckResourceAttr("data.camunda_audit_log.test", "events.0.timestamp", "1700000000000"),
					resource.TestCheckResourceAttr("data.camunda_audit_log.test", "events.0.parent_entity", ""),
					resource.TestCheckResourceAttr("data.camunda_audit_log.test", "events.1.parent_entity", "cluster"),
				),
			},
		},
	})
}
