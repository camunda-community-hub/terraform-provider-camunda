package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	return &managedResource[camundaClusterEncryptionData]{
		typeName: "_cluster_encryption",
		noun:     "cluster encryption",
		schema:   clusterEncryptionSchema,
		describe: func(d camundaClusterEncryptionData) string { return "cluster " + d.ClusterID.ValueString() },
		create: func(op *op, plan camundaClusterEncryptionData) (camundaClusterEncryptionData, error) {
			clusterID := plan.ClusterID.ValueString()
			if err := op.client.WaitClusterHealthy(op.ctx, clusterID); err != nil {
				return plan, err
			}
			if err := op.client.SetClusterEncryptionKeys(op.ctx, clusterID, plan.PrimaryKeyID.ValueString(), plan.SecondaryKeyID.ValueString()); err != nil {
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
			// The key IDs are the ARNs, so a key changed or removed outside
			// Terraform shows as a diff.
			prior.PrimaryKeyID = prior.PrimaryKeyARN
			prior.SecondaryKeyID = prior.SecondaryKeyARN
			return prior, nil
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
			"Creating the resource sets the keys and waits until the encryption is ready. " +
			"**Once applied, the keys can't be changed or removed**: to use other keys, create a new cluster, so changing a key ID is an error. " +
			"Rotating the key material in AWS KMS keeps the ARN and needs nothing here. " +
			"Destroying the resource only removes it from state and leaves the keys configured.",

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
				PlanModifiers:       []planmodifier.String{keyIsImmutable{}},
				MarkdownDescription: "ARN of the AWS KMS key in the cluster's primary region. Single-region and multi-region (`mrk-...`) keys are accepted, as long as the region in the ARN matches the cluster's.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"secondary_key_id": schema.StringAttribute{
				PlanModifiers:       []planmodifier.String{keyIsImmutable{}},
				MarkdownDescription: "ARN of the AWS KMS key in the cluster's backup region. Required for dual-region clusters, must be omitted for single-region clusters, and must differ from `primary_key_id`.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"primary_key_arn":   computed("ARN of the primary key, as the Console reports it."),
			"secondary_key_arn": computed("ARN of the secondary key, as the Console reports it. Null if there is none."),
			"status":            computed("State of the encryption, `Ready` once the keys are in use."),
		},
	}
}

// keyIsImmutable rejects changing a key that is already applied: the Console
// accepts the keys once, and a different key needs a new cluster. Replacing the
// resource wouldn't help, since destroying it leaves the keys on the cluster.
type keyIsImmutable struct{}

func (keyIsImmutable) Description(context.Context) string {
	return "Once applied, a key can't be changed; to use another key, create a new cluster."
}

func (m keyIsImmutable) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }

func (keyIsImmutable) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Creating, destroying, and values not known yet are fine.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if !req.PlanValue.Equal(req.StateValue) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Encryption key can't be changed",
			fmt.Sprintf("The cluster uses %s. The Console doesn't allow changing, adding or removing a key once keys are applied; to use other keys, create a new cluster. Rotating the key material in AWS KMS keeps the ARN.", describeKey(req.StateValue)),
		)
	}
}

func describeKey(key types.String) string {
	if key.IsNull() {
		return "no key here"
	}
	return fmt.Sprintf("%q", key.ValueString())
}
