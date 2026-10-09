package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func organizationMemberConfig(f *fakeConsole, roles string) string {
	return f.providerConfig() + fmt.Sprintf(`
resource "camunda_organization_member" "test" {
  email = "dev@example.com"
  roles = %s
}
`, roles)
}

func TestOrganizationMemberResource(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	expectRoles := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			var got string
			state.do(func(s *consoleState) { got = fmt.Sprint(s.members["dev@example.com"].Roles) })
			if got != want {
				return fmt.Errorf("Console roles = %s, want %s", got, want)
			}
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		CheckDestroy: func(*terraform.State) error {
			return noneLeft(state, func(s *consoleState) int { return len(s.members) }, "members")
		},
		Steps: []resource.TestStep{
			{
				Config: organizationMemberConfig(f, `["developer"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_organization_member.test", "roles.#", "1"),
					expectRoles("[developer]"),
				),
			},
			{
				Config: organizationMemberConfig(f, `["analyst", "developer"]`),
				Check:  expectRoles("[analyst developer]"),
			},
			{
				ResourceName:                         "camunda_organization_member.test",
				ImportState:                          true,
				ImportStateId:                        "dev@example.com",
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "email",
			},
		},
	})
}
