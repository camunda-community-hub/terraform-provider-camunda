package provider

import (
	"testing"
)

func TestCamundaClusterConnectorSecretImportID(t *testing.T) {
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
			data, err := importClusterConnectorSecret(tc.id)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for import ID %q", tc.id)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if data.ClusterId.ValueString() != tc.wantCluster || data.Name.ValueString() != tc.wantName {
				t.Errorf("got cluster_id=%q name=%q, want %q %q", data.ClusterId.ValueString(), data.Name.ValueString(), tc.wantCluster, tc.wantName)
			}
		})
	}
}
