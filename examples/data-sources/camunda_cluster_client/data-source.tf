data "camunda_cluster_client" "worker" {
  cluster_id      = "00000000-0000-0000-0000-000000000000"
  zeebe_client_id = "zeebe-client-id"
}

output "zeebe_address" {
  value = data.camunda_cluster_client.worker.zeebe_address
}
