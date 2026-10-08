package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func clusterData(planType, generation string, autoUpdate, preventDestroy bool) camundaClusterData {
	return camundaClusterData{
		Id:             types.StringValue("id"),
		Name:           types.StringValue("name"),
		Channel:        types.StringValue("channel"),
		Region:         types.StringValue("region"),
		PlanType:       types.StringValue(planType),
		Generation:     types.StringValue(generation),
		AutoUpdate:     types.BoolValue(autoUpdate),
		Description:    types.StringNull(),
		PreventDestroy: types.BoolValue(preventDestroy),
	}
}

func TestCamundaClusterModifyPlan(t *testing.T) {
	ctx := context.Background()
	r := &CamundaClusterResource{}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	s := schemaResp.Schema
	nullRaw := tftypes.NewValue(s.Type().TerraformType(ctx), nil)

	state := func(t *testing.T, d camundaClusterData) tfsdk.State {
		st := tfsdk.State{Schema: s, Raw: nullRaw}
		if diags := st.Set(ctx, &d); diags.HasError() {
			t.Fatal(diags)
		}
		return st
	}
	plan := func(t *testing.T, d camundaClusterData) tfsdk.Plan {
		pl := tfsdk.Plan{Schema: s, Raw: nullRaw}
		if diags := pl.Set(ctx, &d); diags.HasError() {
			t.Fatal(diags)
		}
		return pl
	}

	base := clusterData("plan-1", "gen-1", true, true)

	tests := map[string]struct {
		state       camundaClusterData
		plan        camundaClusterData
		wantErrors  int
		wantReplace []string
	}{
		"no change":                      {state: base, plan: base},
		"name only change":               {state: base, plan: func() camundaClusterData { d := base; d.Name = types.StringValue("new"); return d }()},
		"plan_type change is rejected":   {state: base, plan: clusterData("plan-2", "gen-1", true, true), wantErrors: 1},
		"all unsupported are reported":   {state: base, plan: clusterData("plan-2", "gen-2", false, true), wantErrors: 3},
		"prevent_destroy false replaces": {state: base, plan: clusterData("plan-2", "gen-2", true, false), wantReplace: []string{"plan_type", "generation"}},
		"unknown plan_type is skipped": {
			state: base,
			plan:  func() camundaClusterData { d := base; d.PlanType = types.StringUnknown(); return d }(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resp := &resource.ModifyPlanResponse{Plan: plan(t, tc.plan)}
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: state(t, tc.state), Plan: plan(t, tc.plan)}, resp)

			if got := resp.Diagnostics.ErrorsCount(); got != tc.wantErrors {
				t.Errorf("got %d errors, want %d: %v", got, tc.wantErrors, resp.Diagnostics)
			}
			var got []string
			for _, p := range resp.RequiresReplace {
				got = append(got, p.String())
			}
			if len(got) != len(tc.wantReplace) {
				t.Fatalf("got RequiresReplace %v, want %v", got, tc.wantReplace)
			}
			for i := range got {
				if got[i] != tc.wantReplace[i] {
					t.Errorf("got RequiresReplace %v, want %v", got, tc.wantReplace)
				}
			}
		})
	}

	t.Run("create and destroy are ignored", func(t *testing.T) {
		resp := &resource.ModifyPlanResponse{Plan: plan(t, base)}
		r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: tfsdk.State{Schema: s, Raw: nullRaw}, Plan: plan(t, base)}, resp)
		r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: state(t, base), Plan: tfsdk.Plan{Schema: s, Raw: nullRaw}}, resp)
		if resp.Diagnostics.HasError() || len(resp.RequiresReplace) != 0 {
			t.Errorf("unexpected result: %v %v", resp.Diagnostics, resp.RequiresReplace)
		}
	})
}
