resource "camunda_cluster_secure_connectivity" "test" {
  cluster_id         = camunda_cluster.test.id
  allowed_principals = ["arn:aws:iam::123456789012:root"]
  allowed_regions    = ["eu-west-1"]
}
