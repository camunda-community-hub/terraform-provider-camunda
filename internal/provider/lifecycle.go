package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// managedResource is the lifecycle module: it turns a declaration of how one
// kind of Console object is created, read, updated and deleted into a
// Terraform resource. Resources declare their schema, their model M and the
// hooks below; the lifecycle module owns everything Terraform-specific:
//
//   - Create and Update always end in read, so state reflects the Console
//     rather than the plan.
//   - read returning errNotFound removes the resource from state, and delete
//     returning errNotFound counts as deleted. Nothing else decides this.
//   - Hook errors become diagnostics in one format; hooks only return errors
//     and report warnings through op.Warn.
type managedResource[M any] struct {
	// typeName is appended to the provider type name, e.g. "_cluster".
	typeName string
	// noun names the object in diagnostics, e.g. "cluster".
	noun   string
	schema func() schema.Schema
	// describe identifies an object in diagnostics, e.g. its ID.
	describe func(M) string

	// create creates the object from the plan and returns the model with
	// whatever only the create response carries (IDs, secrets).
	create func(op *op, plan M) (M, error)
	// read returns the object as the Console reports it. prior is the
	// current state, or create's or update's result; read keeps from it what
	// the Console doesn't return.
	read func(op *op, prior M) (M, error)
	// update applies the plan in place. Nil means every attribute forces
	// replacement, so Terraform never calls Update.
	update func(op *op, prior, plan M) (M, error)
	delete func(op *op, prior M) error

	// awaitReady, if set, waits for a created object to become usable. The
	// created object is saved to state first, so a failed wait leaves it
	// tainted instead of orphaned; create must then return no unknown values.
	awaitReady func(op *op, created M) error
	// importID builds the model read needs from an import ID.
	importID func(id string) (M, error)

	client *consoleClient
}

var _ resource.ResourceWithImportState = &managedResource[struct{}]{}
var _ resource.ResourceWithConfigure = &managedResource[struct{}]{}

// op is what a hook gets to work with.
type op struct {
	ctx    context.Context
	client *consoleClient
	diags  *diag.Diagnostics
}

// Warn adds a warning to the operation's diagnostics.
func (o *op) Warn(summary, detail string) {
	o.diags.AddWarning(summary, detail)
}

func (r *managedResource[M]) op(ctx context.Context, diags *diag.Diagnostics) *op {
	return &op{ctx: ctx, client: r.client, diags: diags}
}

func (r *managedResource[M]) fail(diags *diag.Diagnostics, operation string, model M, err error) {
	diags.AddError(
		fmt.Sprintf("Unable to %s %s", operation, r.noun),
		fmt.Sprintf("%s: %s", r.describe(model), err),
	)
}

func (r *managedResource[M]) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + r.typeName
}

func (r *managedResource[M]) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = r.schema()
}

func (r *managedResource[M]) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	client, diags := consoleClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.client = client
}

func (r *managedResource[M]) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	op := r.op(ctx, &resp.Diagnostics)
	created, err := r.create(op, plan)
	if err != nil {
		r.fail(&resp.Diagnostics, "create", plan, err)
		return
	}
	tflog.Info(ctx, "Created "+r.noun, map[string]any{"object": r.describe(created)})

	if r.awaitReady != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &created)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.awaitReady(op, created); err != nil {
			r.fail(&resp.Diagnostics, "create", created, fmt.Errorf("never became ready: %w", err))
			return
		}
	}

	r.readInto(op, "create", created, &resp.State)
}

func (r *managedResource[M]) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior M
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.read(r.op(ctx, &resp.Diagnostics), prior)
	if errors.Is(err, errNotFound) {
		tflog.Info(ctx, r.noun+" no longer exists, removing it from state", map[string]any{"object": r.describe(prior)})
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		r.fail(&resp.Diagnostics, "read", prior, err)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &current)...)
}

func (r *managedResource[M]) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.update == nil {
		resp.Diagnostics.AddError(
			"Unexpected Update",
			"Every attribute of this resource forces replacement, so it can't be updated in place. Please report this issue to the provider developers.",
		)
		return
	}

	var prior, plan M
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	op := r.op(ctx, &resp.Diagnostics)
	updated, err := r.update(op, prior, plan)
	if err != nil {
		r.fail(&resp.Diagnostics, "update", prior, err)
		return
	}

	r.readInto(op, "update", updated, &resp.State)
}

func (r *managedResource[M]) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior M
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.delete(r.op(ctx, &resp.Diagnostics), prior)
	if errors.Is(err, errNotFound) {
		return
	}
	if err != nil {
		r.fail(&resp.Diagnostics, "delete", prior, err)
	}
}

func (r *managedResource[M]) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if r.importID == nil {
		resp.Diagnostics.AddError("Import not supported", fmt.Sprintf("A %s can't be imported.", r.noun))
		return
	}

	imported, err := r.importID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	// Terraform reads the resource right after the import.
	resp.Diagnostics.Append(resp.State.Set(ctx, &imported)...)
}

// readInto finishes a create or update: it reads the object back and saves
// it as state. Here a missing object is an error, not a deletion.
func (r *managedResource[M]) readInto(op *op, operation string, prior M, state *tfsdk.State) {
	current, err := r.read(op, prior)
	if err != nil {
		r.fail(op.diags, operation, prior, fmt.Errorf("reading it back: %w", err))
		return
	}

	op.diags.Append(state.Set(op.ctx, &current)...)
}

// diagsError turns framework diagnostics met inside a hook into an error.
func diagsError(diags diag.Diagnostics) error {
	var errs []error
	for _, d := range diags.Errors() {
		errs = append(errs, fmt.Errorf("%s: %s", d.Summary(), d.Detail()))
	}
	return errors.Join(errs...)
}
