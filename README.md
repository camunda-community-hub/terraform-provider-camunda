# Camunda Platform 8 Terraform Provider 

_Disclaimer: This project is not ready for production usage._

This is an unsupported [Terraform](https://www.terraform.io/) provider for [Camunda Platform 8](https://camunda.com/platform/).
Camunda Platform 8 allows you to *Design, automate, and improve any process across your organization*. 
Further information can be found under https://docs.camunda.io/.

* Documentation: https://registry.terraform.io/providers/multani/camunda/


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
