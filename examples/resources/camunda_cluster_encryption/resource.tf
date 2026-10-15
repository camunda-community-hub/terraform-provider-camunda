data "camunda_channel" "this" {
  name = "Stable"
}

data "camunda_cluster_plan_type" "this" {
  name = "Production"
}

data "camunda_region" "this" {
  name = "Frankfurt, Europe (eu-central-1)"
}

resource "camunda_cluster" "byok" {
  name = "byok"

  channel    = data.camunda_channel.this.id
  generation = data.camunda_channel.this.default_generation_id
  region     = data.camunda_region.this.id
  plan_type  = data.camunda_cluster_plan_type.this.id

  # Required for customer-managed keys.
  encryption = "External"
}

resource "camunda_cluster_encryption" "test" {
  cluster_id     = camunda_cluster.byok.id
  primary_key_id = "arn:aws:kms:eu-central-1:123456789012:key/00000000-0000-0000-0000-000000000000"
}
