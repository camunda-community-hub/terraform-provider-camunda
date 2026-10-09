package provider

import (
	"context"
	"net/http"
	"testing"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func existingCluster() camundaClusterData {
	return camundaClusterData{
		Id:                types.StringValue("c1"),
		Name:              types.StringValue("name"),
		Channel:           types.StringValue("stable"),
		Region:            types.StringValue("eu"),
		PlanType:          types.StringValue("trial"),
		Generation:        types.StringValue("gen-1"),
		AutoUpdate:        types.BoolValue(false),
		CurrentGeneration: types.StringValue("gen-1"),
	}
}

func clusterTFState(t *testing.T, data camundaClusterData) tfsdk.State {
	t.Helper()

	var schemaResp resource.SchemaResponse
	(&CamundaClusterResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(context.Background(), data); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	return state
}

func planCluster(t *testing.T, state, plan camundaClusterData) resource.ModifyPlanResponse {
	t.Helper()

	planned := clusterTFState(t, plan)
	req := resource.ModifyPlanRequest{
		State: clusterTFState(t, state),
		Plan:  tfsdk.Plan{Schema: planned.Schema, Raw: planned.Raw},
	}
	resp := resource.ModifyPlanResponse{Plan: req.Plan}

	(&CamundaClusterResource{}).ModifyPlan(context.Background(), req, &resp)
	return resp
}

func TestClusterPlanAllowsRename(t *testing.T) {
	plan := existingCluster()
	plan.Name = types.StringValue("renamed")

	resp := planCluster(t, existingCluster(), plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got %v", resp.Diagnostics)
	}
}

func TestClusterPlanRejectsImmutableChanges(t *testing.T) {
	tests := map[string]func(*camundaClusterData){
		"plan_type":   func(d *camundaClusterData) { d.PlanType = types.StringValue("production") },
		"auto_update": func(d *camundaClusterData) { d.AutoUpdate = types.BoolValue(true) },
		"generation":  func(d *camundaClusterData) { d.Generation = types.StringValue("gen-2") },
	}

	for attribute, change := range tests {
		t.Run(attribute, func(t *testing.T) {
			plan := existingCluster()
			change(&plan)

			resp := planCluster(t, existingCluster(), plan)

			if resp.Diagnostics.ErrorsCount() != 1 {
				t.Fatalf("expected one error, got %v", resp.Diagnostics)
			}
			withPath, ok := resp.Diagnostics[0].(diag.DiagnosticWithPath)
			if !ok || !withPath.Path().Equal(path.Root(attribute)) {
				t.Fatalf("expected the error on %s, got %v", attribute, resp.Diagnostics)
			}
		})
	}
}

func TestClusterPlanAllowsGenerationChangeWithAutoUpdate(t *testing.T) {
	state := existingCluster()
	state.AutoUpdate = types.BoolValue(true)
	plan := state
	plan.Generation = types.StringValue("gen-2")

	resp := planCluster(t, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got %v", resp.Diagnostics)
	}
}

func TestClusterPlanSkipsCreate(t *testing.T) {
	planned := clusterTFState(t, existingCluster())
	var schemaResp resource.SchemaResponse
	(&CamundaClusterResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	req := resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: schemaResp.Schema},
		Plan:  tfsdk.Plan{Schema: planned.Schema, Raw: planned.Raw},
	}
	resp := resource.ModifyPlanResponse{Plan: req.Plan}

	(&CamundaClusterResource{}).ModifyPlan(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got %v", resp.Diagnostics)
	}
}

func readClusterWithServerGeneration(t *testing.T, autoUpdate bool) camundaClusterData {
	t.Helper()

	f := newFakeConsole(t)
	f.mux.HandleFunc("GET /clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		cluster := clusterWithStatus(r.PathValue("id"), console.CLUSTERCOMPONENTSTATUS_HEALTHY)
		cluster.Generation = console.ClusterGeneration{Uuid: "gen-2"}
		cluster.AutoUpdate = autoUpdate
		writeJSON(t, w, cluster)
	})

	stored := existingCluster()
	stored.AutoUpdate = types.BoolValue(autoUpdate)
	state := clusterTFState(t, stored)
	resp := resource.ReadResponse{State: state}

	(&CamundaClusterResource{client: f.client(t)}).Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics)
	}

	var got camundaClusterData
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("get state: %v", diags)
	}
	return got
}

func TestClusterReadKeepsConfiguredGenerationWithAutoUpdate(t *testing.T) {
	got := readClusterWithServerGeneration(t, true)

	if got.Generation.ValueString() != "gen-1" || got.CurrentGeneration.ValueString() != "gen-2" {
		t.Fatalf("generation = %s, current_generation = %s; want gen-1, gen-2", got.Generation, got.CurrentGeneration)
	}
}

func TestClusterReadReportsGenerationDriftWithoutAutoUpdate(t *testing.T) {
	got := readClusterWithServerGeneration(t, false)

	if got.Generation.ValueString() != "gen-2" || got.CurrentGeneration.ValueString() != "gen-2" {
		t.Fatalf("generation = %s, current_generation = %s; want gen-2, gen-2", got.Generation, got.CurrentGeneration)
	}
}
