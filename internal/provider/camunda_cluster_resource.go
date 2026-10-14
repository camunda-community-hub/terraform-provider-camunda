package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type camundaClusterData struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Channel    types.String `tfsdk:"channel"`
	Region     types.String `tfsdk:"region"`
	PlanType   types.String `tfsdk:"plan_type"`
	Generation types.String `tfsdk:"generation"`
	AutoUpdate types.Bool   `tfsdk:"auto_update"`

	Description       types.String `tfsdk:"description"`
	CurrentGeneration types.String `tfsdk:"current_generation"`

	BackupRegion     types.String `tfsdk:"backup_region"`
	Encryption       types.String `tfsdk:"encryption"`
	StageLabel       types.String `tfsdk:"stage_label"`
	HardwarePackages types.Int64  `tfsdk:"hardware_packages"`

	OwnerID                   types.String `tfsdk:"owner_id"`
	Created                   types.String `tfsdk:"created"`
	Labels                    types.List   `tfsdk:"labels"`
	Status                    types.String `tfsdk:"status"`
	EncryptionStatus          types.String `tfsdk:"encryption_status"`
	EncryptionPrimaryKeyARN   types.String `tfsdk:"encryption_primary_key_arn"`
	EncryptionSecondaryKeyARN types.String `tfsdk:"encryption_secondary_key_arn"`
	Links                     types.Object `tfsdk:"links"`
}

// clusterLinksModel is the value of the links attribute.
type clusterLinksModel struct {
	Zeebe      types.String `tfsdk:"zeebe"`
	Operate    types.String `tfsdk:"operate"`
	Tasklist   types.String `tfsdk:"tasklist"`
	Optimize   types.String `tfsdk:"optimize"`
	Connectors types.String `tfsdk:"connectors"`
	Identity   types.String `tfsdk:"identity"`
	Console    types.String `tfsdk:"console"`
	OAuth      types.String `tfsdk:"oauth"`
}

func NewCamundaClusterResource() resource.Resource {
	return &managedResource[camundaClusterData]{
		typeName: "_cluster",
		noun:     "cluster",
		schema:   clusterSchema,
		describe: func(d camundaClusterData) string { return d.Id.ValueString() },
		create:   createCluster,
		// Creating a cluster takes some time, wait until it's marked healthy.
		awaitReady: func(op *op, created camundaClusterData) error {
			return op.client.WaitClusterHealthy(op.ctx, created.Id.ValueString())
		},
		read:   readCluster,
		update: updateCluster,
		delete: func(op *op, prior camundaClusterData) error {
			return op.client.DeleteCluster(op.ctx, prior.Id.ValueString())
		},
		importID: func(id string) (camundaClusterData, error) {
			return camundaClusterData{
				Id:     types.StringValue(id),
				Labels: types.ListNull(types.StringType),
				Links:  types.ObjectNull(clusterLinksModel{}.attrTypes()),
			}, nil
		},
	}
}

func clusterSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manage a cluster on Camunda SaaS. " +
			"Only `name`, `description`, `stage_label` and `hardware_packages` can be updated in place. Changing `plan_type`, `generation` " +
			"(unless `auto_update` is enabled), `auto_update`, `channel`, `region`, `backup_region` or `encryption` destroys and recreates the cluster, " +
			"**deleting all data of the cluster**. Use `lifecycle { prevent_destroy = true }` to guard against this.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster ID",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster",
				Required:            true,
			},
			"channel": schema.StringAttribute{
				MarkdownDescription: "Channel",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"plan_type": schema.StringAttribute{
				MarkdownDescription: "Plan type. Changing it replaces the cluster.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"generation": schema.StringAttribute{
				MarkdownDescription: "Generation the cluster is created with. Changing it replaces the cluster, unless `auto_update` is enabled: Camunda then upgrades the cluster over time; see `current_generation` for the generation it actually runs.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						generationChangeReplaces,
						"Changing the generation replaces the cluster unless auto_update is enabled.",
						"Changing the generation replaces the cluster unless `auto_update` is enabled.",
					),
				},
			},
			"current_generation": schema.StringAttribute{
				MarkdownDescription: "Generation the cluster currently runs.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"auto_update": schema.BoolAttribute{
				MarkdownDescription: "Auto Update. Changing it replaces the cluster.",
				Optional:            true,
				Default:             booldefault.StaticBool(true),
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the cluster (1 to 150 characters). Remove the attribute to clear it.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 150)},
			},
			"stage_label": schema.StringAttribute{
				MarkdownDescription: "Stage the cluster serves, one of `dev`, `test`, `stage` or `prod`. Can be changed in place. The Console doesn't report it back, so changes made outside Terraform aren't detected, and removing the attribute only stops Terraform from managing it.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.OneOf("dev", "test", "stage", "prod")},
			},
			"hardware_packages": schema.Int64Attribute{
				MarkdownDescription: "Number of hardware packages allocated to the cluster. Only available on request and for Advanced offering cluster types; the Console defaults to 1. Can be changed in place. The Console doesn't report it back, so changes made outside Terraform aren't detected.",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"backup_region": schema.StringAttribute{
				MarkdownDescription: "ID of the region that stores the cluster's backups, see the `camunda_region` data source. Only available if the sales plan supports it (Enterprise). Changing it replaces the cluster.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"encryption": schema.StringAttribute{
				MarkdownDescription: "Type of the key that encrypts the cluster's data: `Software`, `Hardware`, `Provider` or `External`. The Console defaults to `Provider`. Changing it replaces the cluster.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.OneOf("Software", "Hardware", "Provider", "External")},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"encryption_status": schema.StringAttribute{
				MarkdownDescription: "State of the cluster's encryption. Only reported when `encryption` is `External`.",
				Computed:            true,
			},
			"encryption_primary_key_arn": schema.StringAttribute{
				MarkdownDescription: "ARN of the primary customer-managed key. Only reported when `encryption` is `External` and keys were supplied, see `camunda_cluster_encryption`.",
				Computed:            true,
			},
			"encryption_secondary_key_arn": schema.StringAttribute{
				MarkdownDescription: "ARN of the secondary customer-managed key, for dual-region clusters. Only reported when `encryption` is `External` and keys were supplied.",
				Computed:            true,
			},
			"owner_id": schema.StringAttribute{
				MarkdownDescription: "ID of the user or client that owns the cluster.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created": schema.StringAttribute{
				MarkdownDescription: "When the cluster was created, as an RFC 3339 timestamp.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"labels": schema.ListAttribute{
				MarkdownDescription: "Labels of the cluster, as reported by the Console.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Status of the cluster, such as `Healthy` or `Suspended`.",
				Computed:            true,
			},
			"links": schema.SingleNestedAttribute{
				MarkdownDescription: "URLs of the cluster's components. A component the cluster doesn't have has an empty URL.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				Attributes: map[string]schema.Attribute{
					"zeebe":      schema.StringAttribute{Computed: true, MarkdownDescription: "Zeebe gateway URL."},
					"operate":    schema.StringAttribute{Computed: true, MarkdownDescription: "Operate URL."},
					"tasklist":   schema.StringAttribute{Computed: true, MarkdownDescription: "Tasklist URL."},
					"optimize":   schema.StringAttribute{Computed: true, MarkdownDescription: "Optimize URL."},
					"connectors": schema.StringAttribute{Computed: true, MarkdownDescription: "Connectors URL."},
					"identity":   schema.StringAttribute{Computed: true, MarkdownDescription: "Identity URL."},
					"console":    schema.StringAttribute{Computed: true, MarkdownDescription: "URL of the cluster in Console."},
					"oauth":      schema.StringAttribute{Computed: true, MarkdownDescription: "OAuth token URL."},
				},
			},
		},
	}
}

func createCluster(op *op, plan camundaClusterData) (camundaClusterData, error) {
	clusterID, err := op.client.CreateCluster(op.ctx, cluster{
		Name:         plan.Name.ValueString(),
		Description:  plan.Description.ValueString(),
		ChannelID:    plan.Channel.ValueString(),
		RegionID:     plan.Region.ValueString(),
		PlanTypeID:   plan.PlanType.ValueString(),
		GenerationID: plan.Generation.ValueString(),
		AutoUpdate:   plan.AutoUpdate.ValueBool(),

		BackupRegionID:   plan.BackupRegion.ValueString(),
		Encryption:       plan.Encryption.ValueString(),
		StageLabel:       plan.StageLabel.ValueString(),
		HardwarePackages: plan.HardwarePackages.ValueInt64(),
	})
	if err != nil {
		return plan, err
	}

	plan.Id = types.StringValue(clusterID)
	plan.CurrentGeneration = plan.Generation

	// The plan state is saved before waiting for the cluster, so nothing may
	// be unknown; the read after the wait fills these in.
	for _, attr := range []*types.String{
		&plan.BackupRegion, &plan.Encryption, &plan.EncryptionStatus,
		&plan.EncryptionPrimaryKeyARN, &plan.EncryptionSecondaryKeyARN,
		&plan.OwnerID, &plan.Created, &plan.Status,
	} {
		*attr = types.StringNull()
	}
	plan.Labels = types.ListNull(types.StringType)
	plan.Links = types.ObjectNull(clusterLinksModel{}.attrTypes())
	return plan, nil
}

func readCluster(op *op, prior camundaClusterData) (camundaClusterData, error) {
	cluster, err := op.client.GetCluster(op.ctx, prior.Id.ValueString())
	if err != nil {
		return prior, err
	}

	prior.setFromAPI(cluster)
	return prior, nil
}

func updateCluster(op *op, prior, plan camundaClusterData) (camundaClusterData, error) {
	// Every other change forces replacement, and a generation change while
	// auto_update is on needs no API call.
	if plan.Name.Equal(prior.Name) &&
		plan.Description.Equal(prior.Description) &&
		plan.StageLabel.Equal(prior.StageLabel) &&
		plan.HardwarePackages.Equal(prior.HardwarePackages) {
		return plan, nil
	}
	return plan, op.client.UpdateCluster(op.ctx, prior.Id.ValueString(), clusterUpdate{
		Name:             plan.Name.ValueString(),
		Description:      plan.Description.ValueString(),
		StageLabel:       plan.StageLabel.ValueString(),
		HardwarePackages: plan.HardwarePackages.ValueInt64(),
	})
}

// generationChangeReplaces lets the generation change in place while
// auto_update is on: Camunda then moves the cluster to newer generations
// itself, and the configured generation only records where it started.
func generationChangeReplaces(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	var stateAutoUpdate, planAutoUpdate types.Bool
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("auto_update"), &stateAutoUpdate)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("auto_update"), &planAutoUpdate)...)

	resp.RequiresReplace = !stateAutoUpdate.ValueBool() || !planAutoUpdate.ValueBool()
}

// setFromAPI copies what the Console reports about the cluster into the model.
func (d *camundaClusterData) setFromAPI(cl *cluster) {
	d.Name = types.StringValue(cl.Name)
	d.Channel = types.StringValue(cl.ChannelID)
	d.Region = types.StringValue(cl.RegionID)
	d.PlanType = types.StringValue(cl.PlanTypeID)
	d.AutoUpdate = types.BoolValue(cl.AutoUpdate)
	if cl.Description == "" {
		d.Description = types.StringNull()
	} else {
		d.Description = types.StringValue(cl.Description)
	}
	d.CurrentGeneration = types.StringValue(cl.GenerationID)

	// The Console doesn't report the stage label and the hardware packages;
	// the model keeps what was configured.
	d.BackupRegion = nullIfEmpty(cl.BackupRegionID)
	d.Encryption = nullIfEmpty(cl.Encryption)
	d.EncryptionStatus = nullIfEmpty(cl.EncryptionStatus)
	d.EncryptionPrimaryKeyARN = nullIfEmpty(cl.EncryptionPrimaryKeyARN)
	d.EncryptionSecondaryKeyARN = nullIfEmpty(cl.EncryptionSecondaryKeyARN)
	d.OwnerID = types.StringValue(cl.OwnerID)
	d.Created = types.StringValue(cl.Created.Format(time.RFC3339))
	d.Labels = stringList(cl.Labels)
	d.Status = types.StringValue(cl.Status)
	d.Links = clusterLinksModel{
		Zeebe:      types.StringValue(cl.Links.Zeebe),
		Operate:    types.StringValue(cl.Links.Operate),
		Tasklist:   types.StringValue(cl.Links.Tasklist),
		Optimize:   types.StringValue(cl.Links.Optimize),
		Connectors: types.StringValue(cl.Links.Connectors),
		Identity:   types.StringValue(cl.Links.Identity),
		Console:    types.StringValue(cl.Links.Console),
		OAuth:      types.StringValue(cl.Links.OAuth),
	}.object()

	// With auto_update the server moves the cluster to newer generations, so
	// the configured generation is only where it started; keep it instead of
	// reporting a diff the user can't apply.
	if d.Generation.IsNull() || !cl.AutoUpdate {
		d.Generation = d.CurrentGeneration
	}
}

// nullIfEmpty is the Terraform value for an optional string the Console left
// out.
func nullIfEmpty(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func (clusterLinksModel) attrTypes() map[string]attr.Type {
	attrTypes := map[string]attr.Type{}
	for _, name := range []string{"zeebe", "operate", "tasklist", "optimize", "connectors", "identity", "console", "oauth"} {
		attrTypes[name] = types.StringType
	}
	return attrTypes
}

func (m clusterLinksModel) object() types.Object {
	object, diags := types.ObjectValueFrom(context.Background(), m.attrTypes(), m)
	if diags.HasError() {
		// Unreachable: attrTypes mirrors the struct's tags.
		panic(diagsError(diags))
	}
	return object
}

// stringList is a Terraform list of the strings, never null.
func stringList(values []string) types.List {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	return types.ListValueMust(types.StringType, elements)
}
