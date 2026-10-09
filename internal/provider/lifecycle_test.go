package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// The lifecycle module is tested through a test-only "widget" resource kept
// in memory, so these tests don't depend on the fake Console.

type widget struct {
	Id       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Token    types.String `tfsdk:"token"`
	Revision types.Int64  `tfsdk:"revision"`
}

type storedWidget struct {
	name     string
	revision int64
}

type widgetStore struct {
	mu      sync.Mutex
	widgets map[string]storedWidget
	created int
	deletes int

	// readyErr makes awaitReady fail.
	readyErr error
	// goneOnDelete makes delete find the widget already gone.
	goneOnDelete bool
}

func newWidgetStore() *widgetStore {
	return &widgetStore{widgets: map[string]storedWidget{}}
}

func (s *widgetStore) do(f func(s *widgetStore)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

func (s *widgetStore) ids() []string {
	var ids []string
	s.do(func(s *widgetStore) {
		for id := range s.widgets {
			ids = append(ids, id)
		}
	})
	return ids
}

// widgetResource declares a widget. Revision is set only by read, and the
// token only by create, so tests can tell which hook a value came from.
func widgetResource(store *widgetStore) *managedResource[widget] {
	return &managedResource[widget]{
		typeName: "_widget",
		noun:     "widget",
		schema: func() schema.Schema {
			return schema.Schema{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Computed:      true,
						PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"name": schema.StringAttribute{Required: true},
					"token": schema.StringAttribute{
						Computed:      true,
						PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"revision": schema.Int64Attribute{Computed: true},
				},
			}
		},
		describe: func(w widget) string { return "widget " + w.Id.ValueString() },
		create: func(op *op, plan widget) (widget, error) {
			var id string
			store.do(func(s *widgetStore) {
				s.created++
				id = fmt.Sprintf("w%d", s.created)
				s.widgets[id] = storedWidget{name: plan.Name.ValueString(), revision: 1}
			})
			plan.Id = types.StringValue(id)
			plan.Token = types.StringValue("token-" + id)
			plan.Revision = types.Int64Value(0)
			return plan, nil
		},
		read: func(op *op, prior widget) (widget, error) {
			var stored storedWidget
			var ok bool
			store.do(func(s *widgetStore) { stored, ok = s.widgets[prior.Id.ValueString()] })
			if !ok {
				return widget{}, fmt.Errorf("%w: widget %s", errNotFound, prior.Id.ValueString())
			}
			prior.Name = types.StringValue(stored.name)
			prior.Revision = types.Int64Value(stored.revision)
			return prior, nil
		},
		update: func(op *op, prior, plan widget) (widget, error) {
			store.do(func(s *widgetStore) {
				stored := s.widgets[prior.Id.ValueString()]
				stored.name = plan.Name.ValueString()
				stored.revision++
				s.widgets[prior.Id.ValueString()] = stored
			})
			return plan, nil
		},
		delete: func(op *op, prior widget) error {
			if prior.Name.ValueString() == "warn" {
				op.Warn("Widget kept", "widgets named warn are never deleted")
				return nil
			}
			var err error
			store.do(func(s *widgetStore) {
				s.deletes++
				delete(s.widgets, prior.Id.ValueString())
				if s.goneOnDelete {
					err = fmt.Errorf("%w: widget %s", errNotFound, prior.Id.ValueString())
				}
			})
			return err
		},
		awaitReady: func(op *op, created widget) error {
			var err error
			store.do(func(s *widgetStore) { err = s.readyErr })
			return err
		},
		importID: func(id string) (widget, error) {
			if !strings.HasPrefix(id, "w") {
				return widget{}, fmt.Errorf("expected a widget ID, got %q", id)
			}
			return widget{Id: types.StringValue(id)}, nil
		},
	}
}

type widgetProvider struct{ store *widgetStore }

func (p *widgetProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "camunda"
}

func (p *widgetProvider) Schema(context.Context, provider.SchemaRequest, *provider.SchemaResponse) {}

func (p *widgetProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p *widgetProvider) Resources(context.Context) []func() frameworkresource.Resource {
	return []func() frameworkresource.Resource{
		func() frameworkresource.Resource { return widgetResource(p.store) },
	}
}

func (p *widgetProvider) DataSources(context.Context) []func() datasource.DataSource { return nil }

func widgetProviders(store *widgetStore) map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"camunda": providerserver.NewProtocol6WithError(&widgetProvider{store: store}),
	}
}

func widgetConfig(name string) string {
	return fmt.Sprintf(`
resource "camunda_widget" "test" {
  name = %q
}
`, name)
}

func checkWidgets(store *widgetStore, want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		got := strings.Join(store.ids(), ",")
		if got != strings.Join(want, ",") {
			return fmt.Errorf("expected widgets %v in the store, got %q", want, got)
		}
		return nil
	}
}

func TestLifecycle(t *testing.T) {
	store := newWidgetStore()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: widgetProviders(store),
		CheckDestroy:             checkWidgets(store),
		Steps: []resource.TestStep{
			{
				// Create ends in read: revision comes from read, the token
				// only from create.
				Config: widgetConfig("a"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_widget.test", "id", "w1"),
					resource.TestCheckResourceAttr("camunda_widget.test", "revision", "1"),
					resource.TestCheckResourceAttr("camunda_widget.test", "token", "token-w1"),
				),
			},
			{
				// A widget deleted outside Terraform leaves state and is
				// created again.
				PreConfig: func() {
					store.do(func(s *widgetStore) { delete(s.widgets, "w1") })
				},
				Config: widgetConfig("a"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_widget.test", "id", "w2"),
					checkWidgets(store, "w2"),
				),
			},
			{
				// Update ends in read, and read keeps the token from state.
				Config: widgetConfig("b"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camunda_widget.test", "name", "b"),
					resource.TestCheckResourceAttr("camunda_widget.test", "revision", "2"),
					resource.TestCheckResourceAttr("camunda_widget.test", "token", "token-w2"),
				),
			},
			{
				ResourceName:      "camunda_widget.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Only create returns the token.
				ImportStateVerifyIgnore: []string{"token"},
			},
			{
				ResourceName:  "camunda_widget.test",
				ImportState:   true,
				ImportStateId: "nope",
				ExpectError:   regexp.MustCompile(`Invalid import ID`),
			},
		},
	})
}

func TestLifecycleDeleteOfMissingWidgetSucceeds(t *testing.T) {
	store := newWidgetStore()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: widgetProviders(store),
		CheckDestroy: func(*terraform.State) error {
			var deletes int
			store.do(func(s *widgetStore) { deletes = s.deletes })
			if deletes != 1 {
				return fmt.Errorf("expected one delete call, got %d", deletes)
			}
			return nil
		},
		Steps: []resource.TestStep{{
			Config: widgetConfig("a"),
			Check: func(*terraform.State) error {
				store.do(func(s *widgetStore) { s.goneOnDelete = true })
				return nil
			},
		}},
	})
}

func TestLifecycleFailedWaitTaintsTheResource(t *testing.T) {
	store := newWidgetStore()
	store.readyErr = errors.New("still starting")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: widgetProviders(store),
		Steps: []resource.TestStep{
			{
				Config:      widgetConfig("a"),
				ExpectError: regexp.MustCompile(`Unable to create widget(.|\n)*never became ready: still starting`),
			},
			{
				// The tainted widget is in state, so it gets replaced rather
				// than orphaned.
				PreConfig: func() {
					store.do(func(s *widgetStore) { s.readyErr = nil })
				},
				Config: widgetConfig("a"),
				Check:  checkWidgets(store, "w2"),
			},
		},
	})
}

// stateOf builds the Terraform state a framework request carries.
func stateOf(t *testing.T, r frameworkresource.Resource, model any) tfsdk.State {
	t.Helper()
	ctx := context.Background()

	schemaResp := &frameworkresource.SchemaResponse{}
	r.Schema(ctx, frameworkresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}
	if diags := state.Set(ctx, model); diags.HasError() {
		t.Fatalf("building state: %v", diags)
	}
	return state
}

func TestLifecycleHookWarningsReachTerraform(t *testing.T) {
	r := widgetResource(newWidgetStore())
	resp := &frameworkresource.DeleteResponse{}

	r.Delete(context.Background(), frameworkresource.DeleteRequest{
		State: stateOf(t, r, &widget{
			Id: types.StringValue("w1"), Name: types.StringValue("warn"),
			Token: types.StringValue("t"), Revision: types.Int64Value(1),
		}),
	}, resp)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 1 {
		t.Fatalf("expected exactly one warning, got %v", resp.Diagnostics)
	}
}

func TestLifecycleWithoutUpdateRejectsUpdates(t *testing.T) {
	r := widgetResource(newWidgetStore())
	r.update = nil
	resp := &frameworkresource.UpdateResponse{}

	r.Update(context.Background(), frameworkresource.UpdateRequest{}, resp)

	if !resp.Diagnostics.HasError() || resp.Diagnostics[0].Summary() != "Unexpected Update" {
		t.Fatalf("expected an Unexpected Update error, got %v", resp.Diagnostics)
	}
}
