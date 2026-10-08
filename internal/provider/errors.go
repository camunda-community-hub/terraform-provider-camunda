package provider

import (
	"fmt"
	"net/http"

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

// isNotFound reports whether a failed request was answered with HTTP 404.
// The response is nil when the request never reached the server (e.g. network
// errors), so it must be checked before it is dereferenced.
func isNotFound(err error, response *http.Response) bool {
	return err != nil && response != nil && response.StatusCode == http.StatusNotFound
}
