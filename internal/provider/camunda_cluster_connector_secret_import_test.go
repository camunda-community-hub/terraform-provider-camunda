package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestCamundaClusterConnectorSecretImportState(t *testing.T) {
	ctx := context.Background()
	r := &CamundaClusterConnectorSecretResource{}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	tests := map[string]struct {
		id          string
		wantCluster string
		wantName    string
		wantErr     bool
	}{
		"valid":             {id: "cluster-1/my-secret", wantCluster: "cluster-1", wantName: "my-secret"},
		"name with slash":   {id: "cluster-1/a/b", wantCluster: "cluster-1", wantName: "a/b"},
		"name only":         {id: "my-secret", wantErr: true},
		"empty cluster id":  {id: "/my-secret", wantErr: true},
		"empty secret name": {id: "cluster-1/", wantErr: true},
		"empty import id":   {id: "", wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resp := &resource.ImportStateResponse{
				State: tfsdk.State{
					Schema: schemaResp.Schema,
					Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
				},
			}

			r.ImportState(ctx, resource.ImportStateRequest{ID: tc.id}, resp)

			if tc.wantErr {
				if !resp.Diagnostics.HasError() {
					t.Fatalf("expected error for import ID %q", tc.id)
				}
				return
			}
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected error: %v", resp.Diagnostics)
			}

			var data camundaClusterConnectorSecret
			if diags := resp.State.Get(ctx, &data); diags.HasError() {
				t.Fatalf("reading state: %v", diags)
			}
			if data.ClusterId.ValueString() != tc.wantCluster || data.Name.ValueString() != tc.wantName {
				t.Errorf("got cluster_id=%q name=%q, want %q %q", data.ClusterId.ValueString(), data.Name.ValueString(), tc.wantCluster, tc.wantName)
			}
		})
	}
}
