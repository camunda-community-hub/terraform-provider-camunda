action "camunda_cluster_wake" "example" {
  config {
    cluster_id = camunda_cluster.test.id
  }
}

# Run it with:
#   terraform apply -invoke=action.camunda_cluster_wake.example
