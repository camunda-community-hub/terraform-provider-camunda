package validators

import (
	"context"
	"fmt"
	"net"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var _ validator.String = IsIPNetwork{}

// IsIPNetwork checks if a string is a valid IP network.
type IsIPNetwork struct{}

// Description describes the validation in plain text formatting.
func (validator IsIPNetwork) Description(_ context.Context) string {
	return "the string must be a valid IP network"
}

// MarkdownDescription describes the validation in Markdown formatting.
func (validator IsIPNetwork) MarkdownDescription(ctx context.Context) string {
	return validator.Description(ctx)
}

// Validate performs the validation.
func (v IsIPNetwork) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()

	ip := net.ParseIP(value)
	if ip == nil { // Unable to parse the IP address

		_, _, err := net.ParseCIDR(value)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid Network Value",
				fmt.Sprintf("%s", err),
			)
			return
		}
	}
}
