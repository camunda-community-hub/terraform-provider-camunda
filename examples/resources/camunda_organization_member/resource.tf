resource "camunda_organization_member" "example" {
  email = "foo@example.org"
  roles = ["visitor"]
}
