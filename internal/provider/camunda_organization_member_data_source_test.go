package provider

import (
	"regexp"
	"testing"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestOrganizationMemberDataSources(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) {
		s.members["dev@example.com"] = console.Member{
			Email: "dev@example.com", Name: "Dev",
			Roles: []console.OrganizationRole{console.ORGANIZATIONROLE_DEVELOPER, console.ORGANIZATIONROLE_ANALYST},
		}
		s.members["boss@example.com"] = console.Member{
			Email: "boss@example.com", Name: "Boss", InvitePending: true,
			Roles: []console.OrganizationRole{console.ORGANIZATIONROLE_OWNER, console.ORGANIZATIONROLE_ADMIN},
		}
	})

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: f.providerConfig() + `
data "camunda_organization_member" "one" {
  email = "dev@example.com"
}

data "camunda_organization_members" "all" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.camunda_organization_member.one", "name", "Dev"),
					resource.TestCheckResourceAttr("data.camunda_organization_member.one", "roles.#", "2"),
					resource.TestCheckResourceAttr("data.camunda_organization_member.one", "owner", "false"),
					resource.TestCheckResourceAttr("data.camunda_organization_members.all", "members.#", "2"),
					resource.TestCheckResourceAttr("data.camunda_organization_members.all", "members.0.email", "boss@example.com"),
					resource.TestCheckResourceAttr("data.camunda_organization_members.all", "members.0.owner", "true"),
					resource.TestCheckResourceAttr("data.camunda_organization_members.all", "members.0.invite_pending", "true"),
					resource.TestCheckResourceAttr("data.camunda_organization_members.all", "members.0.roles.#", "1"),
					resource.TestCheckResourceAttr("data.camunda_organization_members.all", "members.1.email", "dev@example.com"),
				),
			},
		},
	})
}

func TestOrganizationMemberDataSourceReportsUnknownMember(t *testing.T) {
	f := newFakeConsole(t)
	f.serveConsole(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: f.providerConfig() + `
data "camunda_organization_member" "missing" {
  email = "nobody@example.com"
}
`,
				ExpectError: regexp.MustCompile(`nobody@example.com`),
			},
		},
	})
}
