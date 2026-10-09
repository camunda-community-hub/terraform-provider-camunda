package provider

import (
	"fmt"
)

// clusterConfig is a cluster resource named "test" built from the fake's
// parameters, looked up through the data sources.
func clusterConfig(f *fakeConsole, name, planType string, autoUpdate bool) string {
	return f.providerConfig() + fmt.Sprintf(`
data "camunda_channel" "stable" {
  name = %q
}

data "camunda_region" "belgium" {
  name = %q
}

data "camunda_cluster_plan_type" "plan" {
  name = %q
}

resource "camunda_cluster" "test" {
  name        = %q
  channel     = data.camunda_channel.stable.id
  region      = data.camunda_region.belgium.id
  plan_type   = data.camunda_cluster_plan_type.plan.id
  generation  = data.camunda_channel.stable.default_generation_id
  auto_update = %t
}
`, fakeChannelName, fakeRegionName, planType, name, autoUpdate)
}
