package validators

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestValidatorIPNetwork calls ValidateString to check the validation work as expected.
func TestValidatorIPNetwork(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		value         string
		expectSuccess bool
	}{
		"valid ip address": {
			value:         "127.0.0.1",
			expectSuccess: true,
		},
		"invalid": {
			value:         "foobar",
			expectSuccess: false,
		},
		"valid ip network": {
			value:         "192.168.0.0/24",
			expectSuccess: true,
		},
		"invalid ip network": {
			value:         "192.168.0.0/56",
			expectSuccess: false,
		},
	}

	for name, testCase := range testCases {
		name, testCase := name, testCase

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			req := validator.StringRequest{
				ConfigValue: types.StringValue(testCase.value),
			}
			resp := validator.StringResponse{}

			v := IsIPNetwork{}
			v.ValidateString(ctx, req, &resp)

			if resp.Diagnostics.HasError() == testCase.expectSuccess {
				t.Errorf("Value '%s' should have validated: %v", testCase.value, testCase.expectSuccess)
			}
		})
	}
}
