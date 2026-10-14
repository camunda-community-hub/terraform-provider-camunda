package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type auditLogDataSourceData struct {
	Id     types.String         `tfsdk:"id"`
	Events []auditEventDataItem `tfsdk:"events"`
}

type auditEventDataItem struct {
	Service                  types.String `tfsdk:"service"`
	Audit                    types.String `tfsdk:"audit"`
	AuditType                types.String `tfsdk:"audit_type"`
	Timestamp                types.Int64  `tfsdk:"timestamp"`
	UserID                   types.String `tfsdk:"user_id"`
	Entity                   types.String `tfsdk:"entity"`
	EntityID                 types.String `tfsdk:"entity_id"`
	ParentEntity             types.String `tfsdk:"parent_entity"`
	ParentEntityID           types.String `tfsdk:"parent_entity_id"`
	EntityAttribute          types.String `tfsdk:"entity_attribute"`
	EntityAttributeValueFrom types.String `tfsdk:"entity_attribute_value_from"`
	EntityAttributeValueTo   types.String `tfsdk:"entity_attribute_value_to"`
}

func NewCamundaAuditLogDataSource() datasource.DataSource {
	return &managedDataSource[auditLogDataSourceData]{
		typeName: "_audit_log",
		noun:     "audit log",
		schema:   auditLogSchema,
		read: func(op *op, config auditLogDataSourceData) (auditLogDataSourceData, error) {
			events, err := op.client.GetAuditLog(op.ctx)
			if err != nil {
				return config, err
			}

			config.Id = types.StringValue("audit-log")
			config.Events = []auditEventDataItem{}
			for _, e := range events {
				config.Events = append(config.Events, auditEventDataItem{
					Service:                  types.StringValue(e.Service),
					Audit:                    types.StringValue(e.Audit),
					AuditType:                types.StringValue(e.AuditType),
					Timestamp:                types.Int64Value(e.Timestamp),
					UserID:                   types.StringValue(e.UserID),
					Entity:                   types.StringValue(e.Entity),
					EntityID:                 types.StringValue(e.EntityID),
					ParentEntity:             types.StringValue(e.ParentEntity),
					ParentEntityID:           types.StringValue(e.ParentEntityID),
					EntityAttribute:          types.StringValue(e.EntityAttribute),
					EntityAttributeValueFrom: types.StringValue(e.EntityAttributeValueFrom),
					EntityAttributeValueTo:   types.StringValue(e.EntityAttributeValueTo),
				})
			}
			return config, nil
		},
	}
}

func auditLogSchema() schema.Schema {
	str := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: description}
	}

	return schema.Schema{
		MarkdownDescription: "The activity log of the organization. The Console API returns all events at once and offers no filters; filter them in Terraform. Requires a client with permission to read the activity log.",

		Attributes: map[string]schema.Attribute{
			"id": str("Always `audit-log`."),
			"events": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The activity events.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"service":    str("The service the event belongs to."),
						"audit":      str("What happened."),
						"audit_type": str("The kind of action, such as `create` or `delete`."),
						"timestamp": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "When the event happened, in milliseconds since the Unix epoch.",
						},
						"user_id":                     str("The user or client that caused the event."),
						"entity":                      str("The kind of object the event concerns."),
						"entity_id":                   str("The ID of the object the event concerns."),
						"parent_entity":               str("The kind of the object's parent. Empty if not reported."),
						"parent_entity_id":            str("The ID of the object's parent. Empty if not reported."),
						"entity_attribute":            str("The attribute that changed. Empty if not reported."),
						"entity_attribute_value_from": str("The attribute's value before the change. Empty if not reported."),
						"entity_attribute_value_to":   str("The attribute's value after the change. Empty if not reported."),
					},
				},
			},
		},
	}
}
