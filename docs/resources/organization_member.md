---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "camunda_organization_member Resource - terraform-provider-camunda"
subcategory: ""
description: |-
  Manage a member of an organization
---

# camunda_organization_member (Resource)

Manage a member of an organization

## Example Usage

```terraform
resource "camunda_organization_member" "example" {
  email = "foo@example.org"
  roles = ["visitor"]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `email` (String) The email of the member
- `roles` (Set of String) The roles of this member in the organization. Must be one of: `admin`, `analyst`, `developer`, `operationsengineer`, `taskuser`, or `visitor`.
