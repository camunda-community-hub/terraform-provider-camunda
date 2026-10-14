data "camunda_cluster_backups" "all" {
  cluster_id = "00000000-0000-0000-0000-000000000000"
}

output "completed_backup_ids" {
  value = [
    for backup in data.camunda_cluster_backups.all.backups : backup.id
    if backup.status == "Complete"
  ]
}
