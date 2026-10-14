data "camunda_ip_ranges" "all" {}

output "web_modeler_ip_ranges" {
  value = data.camunda_ip_ranges.all.web_modeler
}
