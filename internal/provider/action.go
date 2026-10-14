package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// managedAction is the action counterpart of managedResource: an action
// declares its schema, its model M and one invoke hook, and gets the Terraform
// plumbing (configure, config handling, one diagnostic format) from here.
//
// Actions run one-off operations that have no state to manage, such as
// upgrading or restoring a cluster. They need Terraform 1.14 or later.
type managedAction[M any] struct {
	// typeName is appended to the provider type name, e.g. "_cluster_wake".
	typeName string
	// verb names what the action does in diagnostics, e.g. "wake cluster".
	verb   string
	schema func() schema.Schema
	// invoke runs the action. progress reports a message to the practitioner
	// while it runs.
	invoke func(op *op, config M, progress func(message string)) error
	// validate, if set, checks the configuration before anything runs. The
	// configuration may hold unknown values, so it is read attribute by
	// attribute rather than into M.
	validate func(ctx context.Context, config tfsdk.Config) diag.Diagnostics

	client *consoleClient
}

var _ action.ActionWithConfigure = &managedAction[struct{}]{}
var _ action.ActionWithValidateConfig = &managedAction[struct{}]{}

func (a *managedAction[M]) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + a.typeName
}

func (a *managedAction[M]) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = a.schema()
}

func (a *managedAction[M]) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	client, diags := consoleClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	a.client = client
}

func (a *managedAction[M]) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	if a.validate != nil {
		resp.Diagnostics.Append(a.validate(ctx, req.Config)...)
	}
}

func (a *managedAction[M]) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config M
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}

	if err := a.invoke(&op{ctx: ctx, client: a.client, diags: &resp.Diagnostics}, config, progress); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to %s", a.verb), err.Error())
	}
}
