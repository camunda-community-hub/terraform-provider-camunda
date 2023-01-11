resource "camunda_cluster_client" "test" {
  name       = "test-client"
  cluster_id = camunda_cluster.test.id
}

output "address" {
  value = camunda_cluster_client.test.zeebe_address
}

output "authorization_server_url" {
  value = camunda_cluster_client.test.zeebe_authorization_server_url
}

output "client_id" {
  value = camunda_cluster_client.test.zeebe_client_id
}

output "client_secret" {
  sensitive = true
  value     = camunda_cluster_client.test.secret
}
