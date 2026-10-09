package provider

import (
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type camundaOrganizationMemberData struct {
	Email types.String `tfsdk:"email"`
	Roles types.Set    `tfsdk:"roles"`
}

func NewCamundaOrganizationMemberResource() resource.Resource {
	return &managedResource[camundaOrganizationMemberData]{
		typeName: "_organization_member",
		noun:     "organization member",
		schema:   organizationMemberSchema,
		describe: func(d camundaOrganizationMemberData) string { return d.Email.ValueString() },
		create:   setOrganizationMemberRoles,
		read:     readOrganizationMember,
		update: func(op *op, prior, plan camundaOrganizationMemberData) (camundaOrganizationMemberData, error) {
			return setOrganizationMemberRoles(op, plan)
		},
		delete: deleteOrganizationMember,
		importID: func(email string) (camundaOrganizationMemberData, error) {
			return camundaOrganizationMemberData{
				Email: types.StringValue(email),
				Roles: types.SetNull(types.StringType),
			}, nil
		},
	}
}

func organizationMemberSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manage a member of an organization.\n\n" +
			"The organization owner's roles can't be changed through the API. For the owner, the provider records `roles` in state without sending them, " +
			"and destroying the resource only removes it from state. Roles that can't be assigned, such as `owner`, are never read into `roles`.",

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
						stringvalidator.OneOf(assignableMemberRoles...),
					),
				},
			},
		},
	}
}

// setOrganizationMemberRoles invites the member if needed and replaces their
// roles. For the owner, whose roles the API won't change, it only warns.
func setOrganizationMemberRoles(op *op, plan camundaOrganizationMemberData) (camundaOrganizationMemberData, error) {
	var roles []string
	if diags := plan.Roles.ElementsAs(op.ctx, &roles, false); diags.HasError() {
		return plan, diagsError(diags)
	}

	err := op.client.SetMemberRoles(op.ctx, plan.Email.ValueString(), roles)
	if errors.Is(err, errOwnerUnchangeable) {
		op.Warn(
			"Organization owner roles not changed",
			fmt.Sprintf("%s is the organization owner, whose roles can't be changed through the API. The roles were recorded in Terraform state but not sent to Camunda.", plan.Email.ValueString()),
		)
		return plan, nil
	}
	return plan, err
}

func readOrganizationMember(op *op, prior camundaOrganizationMemberData) (camundaOrganizationMemberData, error) {
	found, err := op.client.GetMember(op.ctx, prior.Email.ValueString())
	if err != nil {
		return prior, err
	}

	prior.Email = types.StringValue(found.Email)

	// The owner's roles can't be changed, so keep whatever the configuration
	// last recorded. After an import nothing is recorded yet.
	if !found.Owner || prior.Roles.IsNull() {
		roles, diags := types.SetValueFrom(op.ctx, types.StringType, found.Roles)
		if diags.HasError() {
			return prior, diagsError(diags)
		}
		prior.Roles = roles
	}

	return prior, nil
}

func deleteOrganizationMember(op *op, prior camundaOrganizationMemberData) error {
	err := op.client.DeleteMember(op.ctx, prior.Email.ValueString())
	if errors.Is(err, errOwnerUnchangeable) {
		op.Warn(
			"Organization owner not removed",
			fmt.Sprintf("%s is the organization owner, who can't be removed through the API. The member was removed from Terraform state only.", prior.Email.ValueString()),
		)
		return nil
	}
	return err
}
