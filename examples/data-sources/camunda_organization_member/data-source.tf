data "camunda_organization_member" "alice" {
  email = "alice@example.com"
}

output "alice_roles" {
  value = data.camunda_organization_member.alice.roles
}
