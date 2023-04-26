data "camunda_region" "europe" {
  name = "Belgium, Europe (europe-west1)"
}

output "region" {
  value = data.camunda_region.europe.id
}
