resource "camunda_cluster_monitoring" "test" {
  cluster_id = camunda_cluster.test.id
}
