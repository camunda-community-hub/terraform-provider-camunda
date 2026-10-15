package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// managedDataSource is the data source counterpart of managedResource: a data
// source declares its schema, its model M and a read hook, and gets the
// Terraform plumbing (configure, config and state handling, one diagnostic
// format) from here.
type managedDataSource[M any] struct {
	// typeName is appended to the provider type name, e.g. "_cluster".
	typeName string
	// noun names the object in diagnostics, e.g. "cluster".
	noun   string
	schema func() schema.Schema
	// read fills the model from the Console. config holds the attributes the
	// user set; errNotFound is reported like any other error.
	read func(op *op, config M) (M, error)

	client *consoleClient
}

var _ datasource.DataSourceWithConfigure = &managedDataSource[struct{}]{}

func (d *managedDataSource[M]) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + d.typeName
}

func (d *managedDataSource[M]) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = d.schema()
}

func (d *managedDataSource[M]) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	client, diags := consoleClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	d.client = client
}

func (d *managedDataSource[M]) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config M
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data, err := d.read(&op{ctx: ctx, client: d.client, diags: &resp.Diagnostics}, config)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to read %s", d.noun), err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
