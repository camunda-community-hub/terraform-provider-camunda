package provider

import (
	"context"
	"errors"
	"fmt"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &CamundaOrganizationMemberResource{}
var _ resource.ResourceWithImportState = &CamundaOrganizationMemberResource{}

type camundaOrganizationMemberData struct {
	Email types.String `tfsdk:"email"`
	Roles types.Set    `tfsdk:"roles"`
}

type CamundaOrganizationMemberResource struct {
	client *consoleClient
}

func NewCamundaOrganizationMemberResource() resource.Resource {
	return &CamundaOrganizationMemberResource{}
}

func (r *CamundaOrganizationMemberResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_member"
}

func (r *CamundaOrganizationMemberResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a member of an organization",

		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				MarkdownDescription: "The email of the member",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"roles": schema.SetAttribute{
				MarkdownDescription: "The roles of this member in the organization. Must be one of: `admin`, `analyst`, `developer`, `operationsengineer`, `taskuser`, or `visitor`.",
				Required:            true,
				ElementType:         types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf([]string{
							string(console.ORGANIZATIONROLEADMIN_ADMIN),
							string(console.ORGANIZATIONROLEOPERATIONSENGINEER_OPERATIONSENGINEER),
							string(console.ORGANIZATIONROLETASKUSER_TASKUSER),
							string(console.ORGANIZATIONROLEANALYST_ANALYST),
							string(console.ORGANIZATIONROLEDEVELOPER_DEVELOPER),
							string(console.ORGANIZATIONROLEVISITOR_VISITOR),
						}...),
					),
				},
			},
		},
	}
}

func (r *CamundaOrganizationMemberResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Provider not yet configured
	if req.ProviderData == nil {
		return
	}

	client, diags := consoleClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.client = client
}

func (r *CamundaOrganizationMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data camundaOrganizationMemberData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	roles, diags := memberRoles(ctx, data.Roles)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.SetMemberRoles(ctx, data.Email.ValueString(), roles)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to add organization member",
			fmt.Sprintf("Unable to add organization member, got error: %s", err),
		)
		return
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)

	tflog.Info(ctx, "Member added to organization", map[string]interface{}{
		"email": data.Email,
		"roles": data.Roles,
	})
}

func (r *CamundaOrganizationMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data camundaOrganizationMemberData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	member, err := r.client.GetMember(ctx, data.Email.ValueString())
	if errors.Is(err, errNotFound) {
		tflog.Info(ctx, "Member not found", map[string]interface{}{
			"email": data.Email,
		})
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to get organization members, got error: %s", err),
		)
		return
	}

	roles, diags := types.SetValueFrom(ctx, types.StringType, member.Roles)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	data.Email = types.StringValue(member.Email)
	data.Roles = roles

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaOrganizationMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data camundaOrganizationMemberData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	roles, diags := memberRoles(ctx, data.Roles)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.SetMemberRoles(ctx, data.Email.ValueString(), roles)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update organization member",
			fmt.Sprintf("Unable to update organization member, got error: %s", err),
		)
		return
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *CamundaOrganizationMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data camundaOrganizationMemberData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	email := data.Email.ValueString()

	err := r.client.DeleteMember(ctx, email)
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to delete member '%s', got error: %s", email, err),
		)
		return
	}
}

func (r *CamundaOrganizationMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("email"), req, resp)
}

// memberRoles reads the role names out of the roles set.
func memberRoles(ctx context.Context, roles types.Set) ([]string, diag.Diagnostics) {
	var names []string
	diags := roles.ElementsAs(ctx, &names, false)
	return names, diags
}
