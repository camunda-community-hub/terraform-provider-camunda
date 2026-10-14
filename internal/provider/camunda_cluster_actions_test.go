package provider

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// invokeAction runs the action against the fake and returns the progress
// messages it sent and the error diagnostics' summaries and details.
func invokeAction(t *testing.T, a action.Action, client *consoleClient, config map[string]tftypes.Value) (progress, errs []string) {
	t.Helper()
	ctx := context.Background()

	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)

	configureResp := &action.ConfigureResponse{}
	a.(action.ActionWithConfigure).Configure(ctx, action.ConfigureRequest{ProviderData: client}, configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("Configure: %v", configureResp.Diagnostics)
	}

	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("action schema is not an object")
	}
	for name := range objectType.AttributeTypes {
		if _, set := config[name]; !set {
			config[name] = tftypes.NewValue(objectType.AttributeTypes[name], nil)
		}
	}

	resp := &action.InvokeResponse{
		SendProgress: func(event action.InvokeProgressEvent) { progress = append(progress, event.Message) },
	}
	a.Invoke(ctx, action.InvokeRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, config)},
	}, resp)

	for _, d := range resp.Diagnostics.Errors() {
		errs = append(errs, d.Summary()+": "+d.Detail())
	}
	return progress, errs
}

func stringValue(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }

func TestClusterUpgradeAction(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) {
		s.clusters["c1"] = &console.Cluster{Uuid: "c1", Name: "one", Generation: console.ClusterGeneration{Uuid: fakeGeneration1}}
	})
	client := f.client(t)

	progress, errs := invokeAction(t, NewCamundaClusterUpgradeAction(), client, map[string]tftypes.Value{"cluster_id": stringValue("c1")})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(progress) != 2 || !strings.Contains(progress[0], "from generation 8.7 to 8.8") || !strings.Contains(progress[1], "healthy") {
		t.Fatalf("progress = %v", progress)
	}
	state.do(func(s *consoleState) {
		if got := s.clusters["c1"]; got.Generation.Uuid != fakeGeneration2 || got.Status.Ready != console.CLUSTERCOMPONENTSTATUS_HEALTHY {
			t.Errorf("cluster = generation %s, status %s; want %s and healthy", got.Generation.Uuid, got.Status.Ready, fakeGeneration2)
		}
	})

	// Nothing left to upgrade to.
	_, errs = invokeAction(t, NewCamundaClusterUpgradeAction(), client, map[string]tftypes.Value{"cluster_id": stringValue("c1")})
	if len(errs) != 1 || !strings.Contains(errs[0], "Unable to upgrade cluster") || !strings.Contains(errs[0], "no upgrade available") {
		t.Fatalf("errors = %v, want the Console's reason", errs)
	}
}

func TestClusterWakeAction(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) {
		s.clusters["c1"] = &console.Cluster{Uuid: "c1", Status: console.ClusterStatus{Ready: console.CLUSTERCOMPONENTSTATUS_SUSPENDED}}
	})

	progress, errs := invokeAction(t, NewCamundaClusterWakeAction(), f.client(t), map[string]tftypes.Value{"cluster_id": stringValue("c1")})
	if len(errs) != 0 || len(progress) != 2 {
		t.Fatalf("progress = %v, errors = %v; want two progress messages", progress, errs)
	}
	state.do(func(s *consoleState) {
		if got := s.clusters["c1"].Status.Ready; got != console.CLUSTERCOMPONENTSTATUS_HEALTHY {
			t.Errorf("cluster status = %s, want healthy", got)
		}
	})

	_, errs = invokeAction(t, NewCamundaClusterWakeAction(), f.client(t), map[string]tftypes.Value{"cluster_id": stringValue("missing")})
	if len(errs) != 1 || !strings.Contains(errs[0], "cluster missing") {
		t.Fatalf("errors = %v, want one naming the cluster", errs)
	}
}

func TestClusterRestoreAction(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) { s.clusters["c1"] = &console.Cluster{Uuid: "c1"} })
	config := func() map[string]tftypes.Value {
		return map[string]tftypes.Value{"cluster_id": stringValue("c1"), "backup_id": stringValue("backup-1")}
	}

	progress, errs := invokeAction(t, NewCamundaClusterRestoreAction(), f.client(t), config())
	if len(errs) != 0 || len(progress) != 2 {
		t.Fatalf("progress = %v, errors = %v; want two progress messages", progress, errs)
	}
	state.do(func(s *consoleState) {
		if s.restores["backup-1"] != console.RESTORESTATE_COMPLETED {
			t.Errorf("restore state = %s, want completed", s.restores["backup-1"])
		}
		s.failRestores = true
	})

	_, errs = invokeAction(t, NewCamundaClusterRestoreAction(), f.client(t), config())
	if len(errs) != 1 || !regexp.MustCompile(`never completed.*FAILED`).MatchString(strings.ReplaceAll(errs[0], "\n", " ")) {
		t.Fatalf("errors = %v, want the failed state", errs)
	}
}

func TestClusterFailoverAction(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) { s.clusters["c1"] = &console.Cluster{Uuid: "c1"} })

	progress, errs := invokeAction(t, NewCamundaClusterFailoverAction(), f.client(t), map[string]tftypes.Value{
		"cluster_id":        stringValue("c1"),
		"backup_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{stringValue("b1"), stringValue("b2")}),
		"restore_backup_id": stringValue("b2"),
	})
	if len(errs) != 0 || len(progress) != 1 || !strings.Contains(progress[0], "failover-cluster") {
		t.Fatalf("progress = %v, errors = %v; want one message naming the new cluster", progress, errs)
	}
	state.do(func(s *consoleState) {
		got := s.failovers["c1"]
		if !slices.Equal(got.BackupIds, []string{"b1", "b2"}) || got.RestoreBackupId != "b2" {
			t.Errorf("failover request = %+v", got)
		}
	})
}

func TestConsoleClientTriggerFailoverToleratesUnknownResultShapes(t *testing.T) {
	f := newFakeConsole(t)
	f.mux.HandleFunc("POST /clusters/{id}/failover", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"newClusterId":"new","results":{"backupClone":{"unexpected":["shape"]},"restore":"ok"}}`)
	})

	result, err := f.client(t).TriggerFailover(context.Background(), "c1", []string{"b"}, "b")
	if err != nil || result.NewClusterID != "new" || !strings.Contains(result.Results, "unexpected") {
		t.Fatalf("TriggerFailover = %+v, %v; want the new cluster despite the unknown result shape", result, err)
	}
}

func TestClusterFailoverActionReportsFailedStages(t *testing.T) {
	for name, results := range map[string]string{
		"known shape":   `{"backupClone":"ok","restore":{"error":"restore exploded"}}`,
		"unknown shape": `{"backupClone":{"error":"clone exploded","details":["x"]},"restore":"ok"}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeConsole(t)
			f.mux.HandleFunc("POST /clusters/{id}/failover", func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, `{"newClusterId":"new","results":`+results+`}`)
			})

			progress, errs := invokeAction(t, NewCamundaClusterFailoverAction(), f.client(t), map[string]tftypes.Value{
				"cluster_id":        stringValue("c1"),
				"backup_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{stringValue("b1")}),
				"restore_backup_id": stringValue("b1"),
			})
			if len(progress) != 0 {
				t.Errorf("progress = %v, want none for a failed failover", progress)
			}
			if len(errs) != 1 || !strings.Contains(errs[0], "new cluster new") || !strings.Contains(errs[0], "exploded") {
				t.Fatalf("errors = %v, want one naming the new cluster and the failed stage", errs)
			}
		})
	}
}

func TestClusterFailoverActionRejectsNoBackups(t *testing.T) {
	ctx := context.Background()
	a := NewCamundaClusterFailoverAction()
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)

	list, ok := schemaResp.Schema.Attributes["backup_ids"].(actionschema.ListAttribute)
	if !ok {
		t.Fatalf("backup_ids is %T, want a list attribute", schemaResp.Schema.Attributes["backup_ids"])
	}

	for name, backups := range map[string][]attr.Value{
		"none": {},
		"one":  {types.StringValue("b1")},
	} {
		req := validator.ListRequest{Path: path.Root("backup_ids"), ConfigValue: types.ListValueMust(types.StringType, backups)}
		resp := &validator.ListResponse{}
		for _, v := range list.ListValidators() {
			v.ValidateList(ctx, req, resp)
		}
		if got, want := resp.Diagnostics.HasError(), name == "none"; got != want {
			t.Errorf("%s backups: rejected = %v, want %v", name, got, want)
		}
	}
}

func TestClusterFailoverActionRejectsRestoreOutsideBackups(t *testing.T) {
	ctx := context.Background()
	a := NewCamundaClusterFailoverAction()
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	objectType := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	listType := tftypes.List{ElementType: tftypes.String}

	tests := map[string]struct {
		backups   tftypes.Value
		restore   tftypes.Value
		wantError bool
	}{
		"restore among backups": {
			backups: tftypes.NewValue(listType, []tftypes.Value{stringValue("b1"), stringValue("b2")}),
			restore: stringValue("b2"),
		},
		"restore outside backups": {
			backups:   tftypes.NewValue(listType, []tftypes.Value{stringValue("b1"), stringValue("b2")}),
			restore:   stringValue("b3"),
			wantError: true,
		},
		"unknown restore": {
			backups: tftypes.NewValue(listType, []tftypes.Value{stringValue("b1")}),
			restore: tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		},
		"unknown backups": {
			backups: tftypes.NewValue(listType, tftypes.UnknownValue),
			restore: stringValue("b3"),
		},
		"one unknown backup": {
			backups: tftypes.NewValue(listType, []tftypes.Value{stringValue("b1"), tftypes.NewValue(tftypes.String, tftypes.UnknownValue)}),
			restore: stringValue("b3"),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resp := &action.ValidateConfigResponse{}
			a.(action.ActionWithValidateConfig).ValidateConfig(ctx, action.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, map[string]tftypes.Value{
					"cluster_id":        stringValue("c1"),
					"backup_ids":        tc.backups,
					"restore_backup_id": tc.restore,
				})},
			}, resp)

			if got := resp.Diagnostics.HasError(); got != tc.wantError {
				t.Fatalf("rejected = %v (%v), want %v", got, resp.Diagnostics, tc.wantError)
			}
		})
	}
}

func TestClusterRestoreActionReportsSupersededRestore(t *testing.T) {
	f := newFakeConsole(t)
	state := f.serveConsole(t)
	state.do(func(s *consoleState) {
		s.clusters["c1"] = &console.Cluster{Uuid: "c1"}
		// Another restore of the same backup replaces the record the action watches.
		s.supersedeRestores = true
	})

	_, errs := invokeAction(t, NewCamundaClusterRestoreAction(), f.client(t), map[string]tftypes.Value{
		"cluster_id": stringValue("c1"),
		"backup_id":  stringValue("backup-1"),
	})
	if len(errs) != 1 || !strings.Contains(strings.ReplaceAll(errs[0], "\n", " "), "superseded by restore someone-elses") {
		t.Fatalf("errors = %v, want the superseding restore", errs)
	}
}
