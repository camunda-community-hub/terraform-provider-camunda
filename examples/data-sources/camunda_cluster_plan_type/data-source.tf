data "camunda_cluster_plan_type" "basic" {
  name = "Basic"
}

data "camunda_cluster_plan_type" "trial" {
  name = "Trial Cluster"
}

output "plan_type" {
  value = data.camunda_cluster_plan_type.basic
}
