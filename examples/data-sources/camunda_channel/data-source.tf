data "camunda_channel" "alpha" {
  name = "Alpha"
}

data "camunda_channel" "stable" {
  name = "Stable"
}

output "data" {
  value = data.camunda_channel.stable
}

output "generation" {
  value = data.camunda_channel.stable.allowed_generations
}

# Look up a generation ID by its name, for example to pin a cluster to a
# generation other than the channel's default.
output "generation_8_8" {
  value = data.camunda_channel.stable.allowed_generation_ids["8.8"]
}
