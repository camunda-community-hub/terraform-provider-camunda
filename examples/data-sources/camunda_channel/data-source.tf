data "camunda_channel" "alpha" {
  name = "Alpha"
}

output "data" {
  value = data.camunda_channel.alpha
}
