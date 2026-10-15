action "camunda_cluster_failover" "example" {
  config {
    cluster_id        = camunda_cluster.test.id
    backup_ids        = ["backup-1", "backup-2"]
    restore_backup_id = "backup-2"
  }
}

# Run it with:
#   terraform apply -invoke=action.camunda_cluster_failover.example
