package provider

import (
	"context"
	"fmt"

	console "github.com/camunda-community-hub/console-customer-api-go"
	openapi "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	provider *CamundaCloudProvider
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
							string(openapi.ORGANIZATIONROLEADMIN_ADMIN),
							string(openapi.ORGANIZATIONROLEOPERATIONSENGINEER_OPERATIONSENGINEER),
							string(openapi.ORGANIZATIONROLETASKUSER_TASKUSER),
							string(openapi.ORGANIZATIONROLEANALYST_ANALYST),
							string(openapi.ORGANIZATIONROLEDEVELOPER_DEVELOPER),
							string(openapi.ORGANIZATIONROLEVISITOR_VISITOR),
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

	provider, ok := req.ProviderData.(*CamundaCloudProvider)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *incidentio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.provider = provider
}

func (r *CamundaOrganizationMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data camundaOrganizationMemberData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)
	err := setMember(ctx, *r.provider.client, data.Email, data.Roles)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to add organization member",
			fmt.Sprintf("Unable to add organization member, got error: %s", formatClientError(err)),
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

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)
	members, _, err := r.provider.client.DefaultAPI.GetMembers(ctx).Execute()

	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to get organization members, got error: %s", formatClientError(err)),
		)
		return
	}

	searchFor := data.Email.ValueString()

	for _, member := range members {
		if member.Email == searchFor {
			roles, diags := types.SetValueFrom(ctx, types.StringType, member.Roles)
			resp.Diagnostics.Append(diags...)

			if resp.Diagnostics.HasError() {
				return
			}

			data.Email = types.StringValue(member.Email)
			data.Roles = roles

			diags = resp.State.Set(ctx, &data)
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	tflog.Info(ctx, "Member not found", map[string]interface{}{
		"email": data.Email,
	})

	resp.State.RemoveResource(ctx)
}

func (r *CamundaOrganizationMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data camundaOrganizationMemberData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)
	err := setMember(ctx, *r.provider.client, data.Email, data.Roles)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update organization member",
			fmt.Sprintf("Unable to update organization member, got error: %s", formatClientError(err)),
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
	ctx = context.WithValue(ctx, console.ContextAccessToken, r.provider.accessToken)

	_, err := r.provider.client.DefaultAPI.DeleteMember(ctx, email).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to delete member '%s', got error: %s", email, formatClientError(err)),
		)
		return
	}
}

func (r *CamundaOrganizationMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("email"), req, resp)
}


func setMember(ctx context.Context, client openapi.APIClient, email types.String, roles types.Set) error {
	orgRoles := make([]openapi.AssignableOrganizationRoleType, 0)

	for _, r := range roles.Elements() {
		var role openapi.AssignableOrganizationRoleType

		roleName := r.String()
		err := role.UnmarshalJSON([]byte(roleName))

		if err != nil {
			return fmt.Errorf("Unable to read role: %w", err)
		}

		orgRoles = append(orgRoles, role)
	}

	body := console.PostMemberBody{
		OrgRoles: orgRoles,
	}

	_, err := client.DefaultAPI.UpdateMembers(ctx, email.ValueString()).
		PostMemberBody(body).
		Execute()

	if err != nil {
		return fmt.Errorf("Error while calling the update member API: %w", err)
	}

	return nil
}
