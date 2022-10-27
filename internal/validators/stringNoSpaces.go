package validators

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type StringNoSpacesValidator struct {
}

// Description returns a plain text description of the validator's behavior, suitable for a practitioner to understand its impact.
func (v StringNoSpacesValidator) Description(ctx context.Context) string {
	return "string should not contain spaces"
}

// MarkdownDescription returns a markdown formatted description of the validator's behavior, suitable for a practitioner to understand its impact.
func (v StringNoSpacesValidator) MarkdownDescription(ctx context.Context) string {
	return "string should not contain spaces"
}

// Validate runs the main validation logic of the validator, reading configuration data out of `req` and updating `resp` with diagnostics.
func (v StringNoSpacesValidator) Validate(ctx context.Context, req tfsdk.ValidateAttributeRequest, resp *tfsdk.ValidateAttributeResponse) {
	// types.String must be the attr.Value produced by the attr.Type in the schema for this attribute
	// for generic validators, use
	// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/tfsdk#ConvertValue
	// to convert into a known type.
	var str types.String
	diags := tfsdk.ValueAs(ctx, req.AttributeConfig, &str)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	if str.IsUnknown() || str.IsNull() {
		return
	}

	if strings.Contains(str.String(), " ") {
		resp.Diagnostics.AddAttributeError(
			req.AttributePath,
			"Invalid String",
			"String contains spaces",
		)

		return
	}
}
