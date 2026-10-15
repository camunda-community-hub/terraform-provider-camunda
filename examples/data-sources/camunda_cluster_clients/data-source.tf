data "camunda_cluster_clients" "all" {
  cluster_id = "00000000-0000-0000-0000-000000000000"
}

output "client_names" {
  value = [for client in data.camunda_cluster_clients.all.clients : client.name]
}
