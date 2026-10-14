data "camunda_cluster" "existing" {
  id = "00000000-0000-0000-0000-000000000000"
}

output "zeebe_url" {
  value = data.camunda_cluster.existing.links.zeebe
}
