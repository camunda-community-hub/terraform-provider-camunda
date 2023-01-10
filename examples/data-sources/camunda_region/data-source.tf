data "camunda_region" "europe" {
  name = "Europe West"
}

output "region" {
  value = data.camunda_region.europe.id
}
