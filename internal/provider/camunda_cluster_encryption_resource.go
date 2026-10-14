package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type camundaClusterEncryptionData struct {
	Id              types.String `tfsdk:"id"`
	ClusterID       types.String `tfsdk:"cluster_id"`
	PrimaryKeyID    types.String `tfsdk:"primary_key_id"`
	SecondaryKeyID  types.String `tfsdk:"secondary_key_id"`
	PrimaryKeyARN   types.String `tfsdk:"primary_key_arn"`
	SecondaryKeyARN types.String `tfsdk:"secondary_key_arn"`
	Status          types.String `tfsdk:"status"`
}

func NewCamundaClusterEncryptionResource() resource.Resource {
	setKeys := func(op *op, plan camundaClusterEncryptionData) error {
		clusterID := plan.ClusterID.ValueString()
		if err := op.client.WaitClusterHealthy(op.ctx, clusterID); err != nil {
			return err
		}
		return op.client.SetClusterEncryptionKeys(op.ctx, clusterID, plan.PrimaryKeyID.ValueString(), plan.SecondaryKeyID.ValueString())
	}

	return &managedResource[camundaClusterEncryptionData]{
		typeName: "_cluster_encryption",
		noun:     "cluster encryption",
		schema:   clusterEncryptionSchema,
		describe: func(d camundaClusterEncryptionData) string { return "cluster " + d.ClusterID.ValueString() },
		create: func(op *op, plan camundaClusterEncryptionData) (camundaClusterEncryptionData, error) {
			if err := setKeys(op, plan); err != nil {
				return plan, err
			}

			// The plan is saved before waiting for the encryption, so nothing
			// may be unknown; the read after the wait fills these in.
			plan.Id = plan.ClusterID
			plan.PrimaryKeyARN = types.StringNull()
			plan.SecondaryKeyARN = types.StringNull()
			plan.Status = types.StringNull()
			return plan, nil
		},
		awaitReady: func(op *op, created camundaClusterEncryptionData) error {
			return op.client.WaitClusterEncryptionReady(op.ctx, created.ClusterID.ValueString())
		},
		read: func(op *op, prior camundaClusterEncryptionData) (camundaClusterEncryptionData, error) {
			// An import only sets the id, which is the cluster ID.
			prior.ClusterID = prior.Id

			found, err := op.client.GetClusterEncryption(op.ctx, prior.Id.ValueString())
			if err != nil {
				return prior, err
			}

			prior.PrimaryKeyARN = nullIfEmpty(found.PrimaryKeyARN)
			prior.SecondaryKeyARN = nullIfEmpty(found.SecondaryKeyARN)
			prior.Status = nullIfEmpty(found.Status)
			// The key IDs are the ARNs, so a key changed outside Terraform shows as a diff.
			if found.PrimaryKeyARN != "" {
				prior.PrimaryKeyID = types.StringValue(found.PrimaryKeyARN)
			}
			if found.SecondaryKeyARN != "" {
				prior.SecondaryKeyID = types.StringValue(found.SecondaryKeyARN)
			}
			return prior, nil
		},
		// Rotating the keys is the same call as setting them.
		update: func(op *op, prior, plan camundaClusterEncryptionData) (camundaClusterEncryptionData, error) {
			if err := setKeys(op, plan); err != nil {
				return plan, err
			}
			return plan, op.client.WaitClusterEncryptionReady(op.ctx, plan.ClusterID.ValueString())
		},
		// The Console can't remove the keys; they stay configured on the cluster.
		delete: func(op *op, prior camundaClusterEncryptionData) error { return nil },
		importID: func(clusterID string) (camundaClusterEncryptionData, error) {
			return camundaClusterEncryptionData{Id: types.StringValue(clusterID)}, nil
		},
	}
}

func clusterEncryptionSchema() schema.Schema {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: description}
	}

	return schema.Schema{
		MarkdownDescription: "Manage the customer-managed encryption keys (BYOK) of a cluster on Camunda SaaS. " +
			"The cluster must have been created with `encryption = \"External\"`, which requires the Enterprise plan and an AWS region. " +
			"Creating the resource sets the keys and waits until the encryption is ready; changing the keys rotates them. " +
			"The Console can't remove the keys, so destroying the resource only removes it from state and leaves the keys configured.",

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
			"primary_key_id": schema.StringAttribute{
				MarkdownDescription: "ARN of the AWS KMS key in the cluster's primary region. Single-region and multi-region (`mrk-...`) keys are accepted, as long as the region in the ARN matches the cluster's.",
				Required:            true,
			},
			"secondary_key_id": schema.StringAttribute{
				MarkdownDescription: "ARN of the AWS KMS key in the cluster's backup region. Required for dual-region clusters, must be omitted for single-region clusters, and must differ from `primary_key_id`.",
				Optional:            true,
			},
			"primary_key_arn":   computed("ARN of the primary key, as the Console reports it."),
			"secondary_key_arn": computed("ARN of the secondary key, as the Console reports it. Null if there is none."),
			"status":            computed("State of the encryption, `Ready` once the keys are in use."),
		},
	}
}
