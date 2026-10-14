package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type camundaClusterSecureConnectivityData struct {
	Id                types.String `tfsdk:"id"`
	ClusterID         types.String `tfsdk:"cluster_id"`
	AllowedPrincipals types.Set    `tfsdk:"allowed_principals"`
	AllowedRegions    types.Set    `tfsdk:"allowed_regions"`
	ServiceName       types.String `tfsdk:"service_name"`
	PrivateDNSName    types.String `tfsdk:"private_dns_name"`
	EndpointRegion    types.String `tfsdk:"endpoint_region"`
	EndpointType      types.String `tfsdk:"endpoint_type"`
	ConnectionCount   types.Int64  `tfsdk:"connection_count"`
	Conditions        types.List   `tfsdk:"conditions"`
}

var secureConnectivityConditionType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"type":    types.StringType,
	"status":  types.StringType,
	"reason":  types.StringType,
	"message": types.StringType,
}}

func NewCamundaClusterSecureConnectivityResource() resource.Resource {
	return &managedResource[camundaClusterSecureConnectivityData]{
		typeName: "_cluster_secure_connectivity",
		noun:     "cluster secure connectivity",
		schema:   clusterSecureConnectivitySchema,
		describe: func(d camundaClusterSecureConnectivityData) string { return "cluster " + d.ClusterID.ValueString() },
		create: func(op *op, plan camundaClusterSecureConnectivityData) (camundaClusterSecureConnectivityData, error) {
			clusterID := plan.ClusterID.ValueString()
			if err := op.client.WaitClusterHealthy(op.ctx, clusterID); err != nil {
				return plan, err
			}

			var principals, regions []string
			if err := diagsError(plan.AllowedPrincipals.ElementsAs(op.ctx, &principals, false)); err != nil {
				return plan, err
			}
			if err := diagsError(plan.AllowedRegions.ElementsAs(op.ctx, &regions, false)); err != nil {
				return plan, err
			}
			if err := op.client.ActivateSecureConnectivity(op.ctx, clusterID, principals, regions); err != nil {
				return plan, err
			}

			// The plan is saved before waiting for the endpoint, so nothing
			// may be unknown; the read after the wait fills these in.
			plan.Id = plan.ClusterID
			plan.ServiceName = types.StringNull()
			plan.PrivateDNSName = types.StringNull()
			plan.EndpointRegion = types.StringNull()
			plan.EndpointType = types.StringNull()
			plan.ConnectionCount = types.Int64Null()
			plan.Conditions = types.ListNull(secureConnectivityConditionType)
			return plan, nil
		},
		// The Console creates the private endpoint in the background.
		awaitReady: func(op *op, created camundaClusterSecureConnectivityData) error {
			return op.client.WaitSecureConnectivityActive(op.ctx, created.ClusterID.ValueString())
		},
		read: readClusterSecureConnectivity,
		delete: func(op *op, prior camundaClusterSecureConnectivityData) error {
			return op.client.DeactivateSecureConnectivity(op.ctx, prior.ClusterID.ValueString())
		},
		importID: func(clusterID string) (camundaClusterSecureConnectivityData, error) {
			return camundaClusterSecureConnectivityData{
				Id:                types.StringValue(clusterID),
				AllowedPrincipals: types.SetNull(types.StringType),
				AllowedRegions:    types.SetNull(types.StringType),
				Conditions:        types.ListNull(secureConnectivityConditionType),
			}, nil
		},
	}
}

func readClusterSecureConnectivity(op *op, prior camundaClusterSecureConnectivityData) (camundaClusterSecureConnectivityData, error) {
	// An import only sets the id, which is the cluster ID.
	prior.ClusterID = prior.Id

	found, err := op.client.GetSecureConnectivity(op.ctx, prior.Id.ValueString())
	if err != nil {
		return prior, err
	}

	ctx := context.Background()
	var diags error
	prior.AllowedPrincipals, diags = stringSet(ctx, found.AllowedPrincipals)
	if diags != nil {
		return prior, diags
	}
	prior.AllowedRegions, diags = stringSet(ctx, found.AllowedRegions)
	if diags != nil {
		return prior, diags
	}

	prior.ServiceName = nullIfEmpty(found.ServiceName)
	prior.PrivateDNSName = nullIfEmpty(found.PrivateDNSName)
	prior.EndpointRegion = nullIfEmpty(found.EndpointRegion)
	prior.EndpointType = nullIfEmpty(found.EndpointType)
	prior.ConnectionCount = types.Int64Value(found.ConnectionCount)

	conditions := make([]attr.Value, 0, len(found.Conditions))
	for _, c := range found.Conditions {
		conditions = append(conditions, types.ObjectValueMust(secureConnectivityConditionType.AttrTypes, map[string]attr.Value{
			"type":    types.StringValue(c.Type),
			"status":  types.StringValue(c.Status),
			"reason":  types.StringValue(c.Reason),
			"message": types.StringValue(c.Message),
		}))
	}
	prior.Conditions = types.ListValueMust(secureConnectivityConditionType, conditions)
	return prior, nil
}

// stringSet is a Terraform set of the strings, never null.
func stringSet(ctx context.Context, values []string) (types.Set, error) {
	set, diags := types.SetValueFrom(ctx, types.StringType, append([]string{}, values...))
	return set, diagsError(diags)
}

func clusterSecureConnectivitySchema() schema.Schema {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: description}
	}

	return schema.Schema{
		MarkdownDescription: "Manage secure connectivity (a private endpoint) of a cluster on Camunda SaaS. " +
			"Creating the resource activates it and waits until the endpoint exists; destroying it deactivates it. " +
			"The Console can't change the allowed principals or regions of an active endpoint, so changing them replaces the resource. " +
			"Only available if the cluster's plan supports it.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID, which is the cluster ID",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "Cluster ID",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"allowed_principals": schema.SetAttribute{
				MarkdownDescription: "The principals (for example AWS account ARNs) allowed to connect to the endpoint. Changing it replaces the resource.",
				Required:            true,
				ElementType:         types.StringType,
				Validators:          []validator.Set{setvalidator.SizeAtLeast(1)},
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"allowed_regions": schema.SetAttribute{
				MarkdownDescription: "The regions allowed to connect to the endpoint. Changing it replaces the resource.",
				Required:            true,
				ElementType:         types.StringType,
				Validators:          []validator.Set{setvalidator.SizeAtLeast(1)},
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"service_name":     computed("Name of the endpoint service to connect to."),
			"private_dns_name": computed("Private DNS name of the endpoint."),
			"endpoint_region":  computed("Region of the endpoint."),
			"endpoint_type":    computed("Type of the endpoint."),
			"connection_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of connections to the endpoint.",
			},
			"conditions": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Status conditions of the endpoint, as reported by the Console.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type":    computed("Type of the condition."),
						"status":  computed("Status of the condition."),
						"reason":  computed("Machine-readable reason for the condition's status."),
						"message": computed("Human-readable message about the condition."),
					},
				},
			},
		},
	}
}
