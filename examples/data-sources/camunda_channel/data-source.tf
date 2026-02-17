data "camunda_channel" "alpha" {
  name = "Alpha"
}

data "camunda_channel" "stable" {
  name = "Stable"
}

output "data" {
  value = data.camunda_channel.stable
}
