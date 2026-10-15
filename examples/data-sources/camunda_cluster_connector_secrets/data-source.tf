data "camunda_cluster_connector_secrets" "all" {
  cluster_id = "00000000-0000-0000-0000-000000000000"
}

output "secret_names" {
  value = data.camunda_cluster_connector_secrets.all.names
}
