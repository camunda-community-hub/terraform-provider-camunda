[![](https://img.shields.io/badge/Community%20Extension-An%20open%20source%20community%20maintained%20project-FF4700)](https://github.com/camunda-community-hub/community) ![Compatible with: Camunda Platform 8](https://img.shields.io/badge/Compatible%20with-Camunda%20Platform%208-0072Ce) [![](https://img.shields.io/badge/Lifecycle-Incubating-blue)](https://github.com/Camunda-Community-Hub/community/blob/main/extension-lifecycle.md#incubating-)

# Camunda Platform 8 Terraform Provider

This is an community supported [Terraform](https://www.terraform.io/) provider
for [Camunda Platform 8](https://camunda.com/platform/).

Camunda Platform 8 allows you to *Design, automate, and improve any process across your organization*.
Further information can be found under https://docs.camunda.io/.

This Terraform provider allows to manage the resources provided by the Camunda
Platform 8, such as clusters, clients, etc.

* Documentation: https://registry.terraform.io/providers/camunda-community-hub/camunda/


## Development

This Terraform provider is built with the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework).

### Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Developing the Provider

- To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

- To generate or update documentation, run `go generate`.

- To run the full suite of acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```
