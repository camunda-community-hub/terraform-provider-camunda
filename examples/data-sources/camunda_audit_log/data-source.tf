data "camunda_audit_log" "all" {}

output "cluster_creations" {
  value = [
    for event in data.camunda_audit_log.all.events : event
    if event.entity == "cluster" && event.audit_type == "c"
  ]
}
