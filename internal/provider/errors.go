package provider

import (
	"fmt"

	console "github.com/camunda-community-hub/console-customer-api-go"
)

func formatClientError(err error) string {
	switch e := err.(type) {
	case (*console.GenericOpenAPIError):
		return fmt.Sprintf("%s: %s", e.Error(), e.Body())

	default:
		return e.Error()
	}
}
