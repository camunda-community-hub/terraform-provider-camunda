---
page_title: "camunda_cluster_client Resource - terraform-provider-camunda"
subcategory: ""
description: |-
    Manage a cluster client on Camunda SaaS.
---

# camunda_cluster_client (Resource)

Manage a cluster client on Camunda SaaS.

## Example Usage

```terraform
resource "camunda_cluster_client" "test" {
  name       = "test-client"
  cluster_id = camunda_cluster.test.id

  scopes = [
    "Operate",
    #"Optimize",
    #"Tasklist",
    "Zeebe",
  ]
}

output "address" {
  value = camunda_cluster_client.test.zeebe_address
}

output "authorization_server_url" {
  value = camunda_cluster_client.test.zeebe_authorization_server_url
}

output "client_id" {
  value = camunda_cluster_client.test.zeebe_client_id
}

output "client_secret" {
  sensitive = true
  value     = camunda_cluster_client.test.secret
}

output "scopes" {
  value = camunda_cluster_client.test.scopes
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `cluster_id` (String) Cluster ID
- `name` (String) The name of the cluster client

### Optional

- `scopes` (Set of String) The list of scopes the client will be valid for. It defaults to all the scopes, and at least one scope should be specified. Valid values:
  * `Operate`
  * `Optimize`
  * `Tasklist`
  * `Zeebe`

### Read-Only

- `id` (String) Cluster Client ID
- `secret` (String, Sensitive) The client secret
- `zeebe_address` (String) Zeebe Address
- `zeebe_authorization_server_url` (String) Zeebe Authorization Server Url
- `zeebe_client_id` (String) Zeebe Client Id
