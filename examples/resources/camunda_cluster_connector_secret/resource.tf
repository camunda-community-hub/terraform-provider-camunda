resource "camunda_cluster_connector_secret" "test" {
  cluster_id = camunda_cluster.test.id
  name       = "test"
  key        = "key-of-secret"
  value      = "my-secret-value12354"
}
