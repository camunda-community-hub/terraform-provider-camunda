package provider

import (
	"fmt"
	"slices"
	"testing"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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

func TestOrganizationMemberResourceIgnoresUnassignableRoles(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) {
		s.members["dev@example.com"] = console.Member{
			Email: "dev@example.com",
			Roles: []console.OrganizationRole{console.ORGANIZATIONROLE_DEVELOPER, console.ORGANIZATIONROLE_MODELER},
		}
	})

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:             organizationMemberConfig(f, `["developer"]`),
				ResourceName:       "camunda_organization_member.test",
				ImportState:        true,
				ImportStateId:      "dev@example.com",
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if got := states[0].Attributes["roles.#"]; got != "1" {
						return fmt.Errorf("imported %s roles, want only developer", got)
					}
					return nil
				},
			},
			{
				Config: organizationMemberConfig(f, `["developer"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func organizationOwnerConfig(f *fakeConsole, roles string) string {
	return f.providerConfig() + fmt.Sprintf(`
resource "camunda_organization_member" "owner" {
  email = "owner@example.com"
  roles = %s
}
`, roles)
}

func TestOrganizationMemberResourceLeavesOwnerAlone(t *testing.T) {
	ownerRoles := []console.OrganizationRole{console.ORGANIZATIONROLE_OWNER, console.ORGANIZATIONROLE_ADMIN}
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) {
		s.members["owner@example.com"] = console.Member{Email: "owner@example.com", Roles: ownerRoles}
	})
	ownerUnchanged := func(*terraform.State) error {
		var got []console.OrganizationRole
		state.do(func(s *consoleState) { got = s.members["owner@example.com"].Roles })
		if !slices.Equal(got, ownerRoles) {
			return fmt.Errorf("owner roles = %v, want %v", got, ownerRoles)
		}
		return nil
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: f.providerFactories(),
		// The owner can't be removed, so destroy only drops it from state.
		CheckDestroy: ownerUnchanged,
		Steps: []resource.TestStep{
			{
				Config:             organizationOwnerConfig(f, `["admin"]`),
				ResourceName:       "camunda_organization_member.owner",
				ImportState:        true,
				ImportStateId:      "owner@example.com",
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if got := states[0].Attributes["roles.#"]; got != "1" {
						return fmt.Errorf("imported %s roles, want only admin", got)
					}
					return nil
				},
			},
			{
				// The owner's assignable roles match the config, so nothing changes.
				Config: organizationOwnerConfig(f, `["admin"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// A differing config applies without calling the API and then
				// plans no further change.
				Config: organizationOwnerConfig(f, `["admin", "analyst"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_organization_member.owner", "roles.#", "2"),
					ownerUnchanged,
				),
			},
		},
	})
}
