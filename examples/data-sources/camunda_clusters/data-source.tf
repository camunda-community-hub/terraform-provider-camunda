data "camunda_clusters" "all" {}

output "healthy_clusters" {
  value = [
    for cluster in data.camunda_clusters.all.clusters : cluster.name
    if cluster.status == "Healthy"
  ]
}
