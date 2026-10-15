data "camunda_organization_members" "all" {}

output "admins" {
  value = [
    for m in data.camunda_organization_members.all.members : m.email
    if contains(m.roles, "admin")
  ]
}
