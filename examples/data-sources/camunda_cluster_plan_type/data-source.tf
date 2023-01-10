data "camunda_cluster_plan_type" "trial_package" {
  name = "Trial Package"
}

output "plan_type" {
  value = data.camunda_cluster_plan_type.trial_package
}
