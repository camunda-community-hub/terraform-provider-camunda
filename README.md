[![](https://img.shields.io/badge/Community%20Extension-An%20open%20source%20community%20maintained%20project-FF4700)](https://github.com/camunda-community-hub/community) ![Compatible with: Camunda Platform 8](https://img.shields.io/badge/Compatible%20with-Camunda%20Platform%208-0072Ce) [![](https://img.shields.io/badge/Lifecycle-Incubating-blue)](https://github.com/Camunda-Community-Hub/community/blob/main/extension-lifecycle.md#incubating-)

# Camunda 8 Terraform Provider

A community-maintained [Terraform](https://www.terraform.io/) provider for
[Camunda 8 SaaS](https://camunda.com/platform/). It manages clusters, cluster API
clients, connector secrets, IP allowlists and organization members through the
[Administration API](https://docs.camunda.io/docs/apis-tools/administration-api/administration-api-reference/).

* Documentation: https://registry.terraform.io/providers/camunda-community-hub/camunda/latest/docs
* Release notes: https://github.com/camunda-community-hub/terraform-provider-camunda/releases
* Upgrading from v0.0.x: [v0.1 upgrade guide](https://registry.terraform.io/providers/camunda-community-hub/camunda/latest/docs/guides/upgrading-to-0.1)

## Quick start

1. In Camunda Hub, go to **Organization > Manage organization > Administration API** and
   create credentials with the scopes for the resources you want to manage (for example **Cluster**).
1. Create a cluster:

   ```terraform
   terraform {
     required_providers {
       camunda = {
         source  = "camunda-community-hub/camunda"
         version = "~> 0.1"
       }
     }
   }

   variable "camunda_client_id" {}
   variable "camunda_client_secret" {
     sensitive = true
   }

   provider "camunda" {
     client_id     = var.camunda_client_id
     client_secret = var.camunda_client_secret
   }

   data "camunda_channel" "stable" {
     name = "Stable"
   }

   data "camunda_region" "belgium" {
     name = "Belgium, Europe (europe-west1)"
   }

   data "camunda_cluster_plan_type" "trial" {
     name = "Trial Cluster"
   }

   resource "camunda_cluster" "dev" {
     name       = "dev"
     channel    = data.camunda_channel.stable.id
     generation = data.camunda_channel.stable.default_generation_id
     region     = data.camunda_region.belgium.id
     plan_type  = data.camunda_cluster_plan_type.trial.id

     lifecycle {
       prevent_destroy = true
     }
   }
   ```

1. Run it, passing the credentials through the environment:

   ```shell
   export TF_VAR_camunda_client_id="<client-id>"
   export TF_VAR_camunda_client_secret="<client-secret>"
   terraform init
   terraform apply
   ```

Changing a cluster's `plan_type`, `generation` or `auto_update` replaces the cluster, which
deletes its data. `prevent_destroy` makes such a plan fail instead, so keep it on clusters you
can't lose.

## Development

This Terraform provider is built with the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework).

### Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

### Developing the Provider

- To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

- To generate or update documentation, run `go generate`.

- To run the tests, run `go test ./internal/...`. They run real Terraform configurations
  against an in-memory fake of the Administration API, so they need neither credentials
  nor a Camunda account.

### Release the Provider

Create a GitHub release with a new `vX.Y.Z` tag, for example with
`gh release create vX.Y.Z --notes-file notes.md`. Pushing a `vX.Y.Z` tag works too.

The tag starts the `Release` workflow, which builds and signs the artifacts with GoReleaser
and attaches them to the release. The Terraform Registry picks up the new version from there.
