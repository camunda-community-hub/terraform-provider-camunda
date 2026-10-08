package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The management API cannot change these attributes in place. They must force replacement so that
// Terraform's lifecycle { prevent_destroy = true } can guard against it, while name and description
// must stay updatable in place.
func TestCamundaClusterReplaceAttributes(t *testing.T) {
	ctx := context.Background()
	r := &CamundaClusterResource{}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	s := schemaResp.Schema
	nullRaw := tftypes.NewValue(s.Type().TerraformType(ctx), nil)

	base := camundaClusterData{
		Id:          types.StringValue("id"),
		Name:        types.StringValue("name"),
		Channel:     types.StringValue("channel"),
		Region:      types.StringValue("region"),
		PlanType:    types.StringValue("plan-1"),
		Generation:  types.StringValue("gen-1"),
		AutoUpdate:  types.BoolValue(true),
		Description: types.StringNull(),
	}

	tests := map[string]struct {
		change      func(d *camundaClusterData)
		wantReplace bool
	}{
		"plan_type":   {func(d *camundaClusterData) { d.PlanType = types.StringValue("plan-2") }, true},
		"generation":  {func(d *camundaClusterData) { d.Generation = types.StringValue("gen-2") }, true},
		"auto_update": {func(d *camundaClusterData) { d.AutoUpdate = types.BoolValue(false) }, true},
		"channel":     {func(d *camundaClusterData) { d.Channel = types.StringValue("channel-2") }, true},
		"region":      {func(d *camundaClusterData) { d.Region = types.StringValue("region-2") }, true},
		"name":        {func(d *camundaClusterData) { d.Name = types.StringValue("new") }, false},
		"description": {func(d *camundaClusterData) { d.Description = types.StringValue("new") }, false},
	}

	for attr, tc := range tests {
		t.Run(attr, func(t *testing.T) {
			planData := base
			tc.change(&planData)

			state := tfsdk.State{Schema: s, Raw: nullRaw}
			plan := tfsdk.Plan{Schema: s, Raw: nullRaw}
			config := tfsdk.Config{Schema: s, Raw: nullRaw}
			if diags := state.Set(ctx, &base); diags.HasError() {
				t.Fatal(diags)
			}
			if diags := plan.Set(ctx, &planData); diags.HasError() {
				t.Fatal(diags)
			}
			config.Raw = plan.Raw

			p := path.Root(attr)
			got := false
			switch a := s.Attributes[attr].(type) {
			case schema.StringAttribute:
				var sv, pv types.String
				state.GetAttribute(ctx, p, &sv)
				plan.GetAttribute(ctx, p, &pv)
				resp := &planmodifier.StringResponse{}
				for _, m := range a.PlanModifiers {
					m.PlanModifyString(ctx, planmodifier.StringRequest{
						Path: p, State: state, Plan: plan, Config: config,
						StateValue: sv, PlanValue: pv, ConfigValue: pv,
					}, resp)
				}
				got = resp.RequiresReplace
			case schema.BoolAttribute:
				var sv, pv types.Bool
				state.GetAttribute(ctx, p, &sv)
				plan.GetAttribute(ctx, p, &pv)
				resp := &planmodifier.BoolResponse{}
				for _, m := range a.PlanModifiers {
					m.PlanModifyBool(ctx, planmodifier.BoolRequest{
						Path: p, State: state, Plan: plan, Config: config,
						StateValue: sv, PlanValue: pv, ConfigValue: pv,
					}, resp)
				}
				got = resp.RequiresReplace
			default:
				t.Fatalf("unexpected attribute type for %s: %T", attr, a)
			}
			if got != tc.wantReplace {
				t.Errorf("RequiresReplace for %s = %v, want %v", attr, got, tc.wantReplace)
			}
		})
	}
}
