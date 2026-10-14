resource "camunda_cluster_monitoring" "test" {
  cluster_id = camunda_cluster.test.id
}

resource "camunda_cluster_monitoring_client" "test" {
  cluster_id = camunda_cluster.test.id
  username   = "prometheus"

  # Change this value to rotate the password.
  rotation_trigger = "2026-01"

  depends_on = [camunda_cluster_monitoring.test]
}

output "prometheus_password" {
  value     = camunda_cluster_monitoring_client.test.password
  sensitive = true
}
