resource "camunda_cluster_backup" "test" {
  cluster_id = camunda_cluster.test.id
}
