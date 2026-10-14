action "camunda_cluster_upgrade" "example" {
  config {
    cluster_id = camunda_cluster.test.id
  }
}

# Run it with:
#   terraform apply -invoke=action.camunda_cluster_upgrade.example
