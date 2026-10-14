data "camunda_cluster_backups" "all" {
  cluster_id = camunda_cluster.test.id
}

action "camunda_cluster_restore" "example" {
  config {
    cluster_id = camunda_cluster.test.id
    backup_id  = data.camunda_cluster_backups.all.backups[0].id
  }
}

# Run it with:
#   terraform apply -invoke=action.camunda_cluster_restore.example
