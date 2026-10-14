package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// The data source module is tested through a test-only "widget" data source
// over the in-memory widget store of the lifecycle tests.

type widgetData struct {
	Name     types.String `tfsdk:"name"`
	Id       types.String `tfsdk:"id"`
	Revision types.Int64  `tfsdk:"revision"`
}

// widgetDataSource finds a widget by name. Looking up "broken" fails, and
// "warn" succeeds with a warning, so tests can tell how hook diagnostics
// reach Terraform.
func widgetDataSource(store *widgetStore) *managedDataSource[widgetData] {
	return &managedDataSource[widgetData]{
		typeName: "_widget",
		noun:     "widget",
		schema: func() schema.Schema {
			return schema.Schema{
				Attributes: map[string]schema.Attribute{
					"name":     schema.StringAttribute{Required: true},
					"id":       schema.StringAttribute{Computed: true},
					"revision": schema.Int64Attribute{Computed: true},
				},
			}
		},
		read: func(op *op, config widgetData) (widgetData, error) {
			switch config.Name.ValueString() {
			case "broken":
				return config, fmt.Errorf("the widget shelf is on fire")
			case "warn":
				op.Warn("Widget is old", "this widget has been around")
			}

			var found bool
			store.do(func(s *widgetStore) {
				for id, stored := range s.widgets {
					if stored.name == config.Name.ValueString() {
						config.Id = types.StringValue(id)
						config.Revision = types.Int64Value(stored.revision)
						found = true
					}
				}
			})
			if !found {
				return config, fmt.Errorf("%w: widget %q", errNotFound, config.Name.ValueString())
			}
			return config, nil
		},
	}
}

func widgetDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "camunda_widget" "w" {
  name = %[1]q
}

data "camunda_widget" "w" {
  name       = %[1]q
  depends_on = [camunda_widget.w]
}
`, name)
}

func TestDataSourceModuleReadsIntoState(t *testing.T) {
	store := newWidgetStore()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: widgetProviders(store),
		Steps: []resource.TestStep{
			{
				Config: widgetDataSourceConfig("gear"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.camunda_widget.w", "id", "camunda_widget.w", "id"),
					resource.TestCheckResourceAttr("data.camunda_widget.w", "name", "gear"),
					resource.TestCheckResourceAttr("data.camunda_widget.w", "revision", "1"),
				),
			},
		},
	})
}

func TestDataSourceModuleTurnsHookErrorsIntoDiagnostics(t *testing.T) {
	store := newWidgetStore()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: widgetProviders(store),
		Steps: []resource.TestStep{
			{
				Config:      widgetDataSourceConfig("broken"),
				ExpectError: regexp.MustCompile(`Unable to read widget[\s\S]*the widget shelf is on fire`),
			},
			{
				// A missing object is reported like any other error.
				Config:      `data "camunda_widget" "w" { name = "ghost" }`,
				ExpectError: regexp.MustCompile(`Unable to read widget[\s\S]*not found`),
			},
		},
	})
}

func TestDataSourceModuleReportsHookWarnings(t *testing.T) {
	store := newWidgetStore()
	store.widgets["w1"] = storedWidget{name: "warn", revision: 3}
	ctx := context.Background()

	ds := widgetDataSource(store)
	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	objectType := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	config := tftypes.NewValue(objectType, map[string]tftypes.Value{
		"name":     tftypes.NewValue(tftypes.String, "warn"),
		"id":       tftypes.NewValue(tftypes.String, nil),
		"revision": tftypes.NewValue(tftypes.Number, nil),
	})
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, nil)}}
	ds.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}
	if got := resp.Diagnostics.WarningsCount(); got != 1 || resp.Diagnostics[0].Summary() != "Widget is old" {
		t.Fatalf("diagnostics = %v, want the hook's warning", resp.Diagnostics)
	}

	var state widgetData
	resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
	if state.Id.ValueString() != "w1" || state.Revision.ValueInt64() != 3 {
		t.Fatalf("state = %+v, want the widget despite the warning", state)
	}
}
