package provider

import (
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationMemberDataSourceData struct {
	Email         types.String   `tfsdk:"email"`
	Name          types.String   `tfsdk:"name"`
	Roles         []types.String `tfsdk:"roles"`
	Owner         types.Bool     `tfsdk:"owner"`
	InvitePending types.Bool     `tfsdk:"invite_pending"`
}

type organizationMembersDataSourceData struct {
	Id      types.String                       `tfsdk:"id"`
	Members []organizationMemberDataSourceData `tfsdk:"members"`
}

func newOrganizationMemberDataSourceData(m member) organizationMemberDataSourceData {
	return organizationMemberDataSourceData{
		Email:         types.StringValue(m.Email),
		Name:          types.StringValue(m.Name),
		Roles:         stringValues(m.AllRoles),
		Owner:         types.BoolValue(m.Owner),
		InvitePending: types.BoolValue(m.InvitePending),
	}
}

func NewCamundaOrganizationMemberDataSource() datasource.DataSource {
	return &managedDataSource[organizationMemberDataSourceData]{
		typeName: "_organization_member",
		noun:     "organization member",
		schema: func() schema.Schema {
			attributes := organizationMemberAttributes()
			attributes["email"] = schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The email of the member.",
			}
			return schema.Schema{
				MarkdownDescription: "Read a member of the organization. Fails if there is no member, invited or active, with that email.",
				Attributes:          attributes,
			}
		},
		read: func(op *op, config organizationMemberDataSourceData) (organizationMemberDataSourceData, error) {
			found, err := op.client.GetMemberDetails(op.ctx, config.Email.ValueString())
			if err != nil {
				return config, err
			}
			return newOrganizationMemberDataSourceData(*found), nil
		},
	}
}

func NewCamundaOrganizationMembersDataSource() datasource.DataSource {
	return &managedDataSource[organizationMembersDataSourceData]{
		typeName: "_organization_members",
		noun:     "organization members",
		schema: func() schema.Schema {
			attributes := organizationMemberAttributes()
			attributes["email"] = schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The email of the member.",
			}
			return schema.Schema{
				MarkdownDescription: "Read all members of the organization, including members whose invitation is pending, sorted by email.",
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Always `organization-members`.",
					},
					"members": schema.ListNestedAttribute{
						Computed:            true,
						MarkdownDescription: "The members of the organization.",
						NestedObject:        schema.NestedAttributeObject{Attributes: attributes},
					},
				},
			}
		},
		read: func(op *op, config organizationMembersDataSourceData) (organizationMembersDataSourceData, error) {
			listed, err := op.client.ListMembers(op.ctx)
			if err != nil {
				return config, err
			}
			slices.SortFunc(listed, func(a, b member) int { return strings.Compare(a.Email, b.Email) })

			config.Id = types.StringValue("organization-members")
			config.Members = make([]organizationMemberDataSourceData, 0, len(listed))
			for _, m := range listed {
				config.Members = append(config.Members, newOrganizationMemberDataSourceData(m))
			}
			return config, nil
		},
	}
}

// organizationMemberAttributes are the computed attributes both member data
// sources share; the callers add email, which one requires and the other
// reports.
func organizationMemberAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The name of the member.",
		},
		"roles": schema.ListAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "All roles of the member as the Console reports them, including roles that can't be assigned through the API, such as `owner`.",
		},
		"owner": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether the member is the organization owner, whose roles can't be changed through the API. Convenience for `roles` containing `owner`.",
		},
		"invite_pending": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether the member hasn't accepted the invitation yet.",
		},
	}
}
