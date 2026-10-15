action "camunda_cluster_upgrade" "example" {
  config {
    cluster_id = camunda_cluster.test.id
  }
}

# For a cluster with auto_update = false, set generation on the camunda_cluster
# resource to the generation it now runs afterwards. Until then the next apply
# plans to replace the cluster and delete its data, so guard it with
# lifecycle { prevent_destroy = true } and check the plan.
#
# Run it with:
#   terraform apply -invoke=action.camunda_cluster_upgrade.example
