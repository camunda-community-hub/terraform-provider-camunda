resource "camunda_cluster_connector_secret" "test" {
  cluster_id = camunda_cluster.test.id
  name       = "my-key-of-secret"
  value      = "my-secret-value"
}
