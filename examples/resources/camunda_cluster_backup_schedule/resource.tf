resource "camunda_cluster_backup_schedule" "test" {
  cluster_id = camunda_cluster.test.id

  # Every day at 02:00 UTC.
  schedule = "0 2 * * *"
}
