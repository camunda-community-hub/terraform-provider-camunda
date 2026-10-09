package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
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
			return camundaClusterData{Id: types.StringValue(id)}, nil
		},
	}
}

func clusterSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manage a cluster on Camunda SaaS. " +
			"Only `name` and `description` can be updated in place. Changing `plan_type`, `generation` " +
			"(unless `auto_update` is enabled), `auto_update`, `channel` or `region` destroys and recreates the cluster, " +
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
	})
	if err != nil {
		return plan, err
	}

	plan.Id = types.StringValue(clusterID)
	plan.CurrentGeneration = plan.Generation
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
	if plan.Name.Equal(prior.Name) && plan.Description.Equal(prior.Description) {
		return plan, nil
	}
	return plan, op.client.UpdateCluster(op.ctx, prior.Id.ValueString(), plan.Name.ValueString(), plan.Description.ValueString())
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

	// With auto_update the server moves the cluster to newer generations, so
	// the configured generation is only where it started; keep it instead of
	// reporting a diff the user can't apply.
	if d.Generation.IsNull() || !cl.AutoUpdate {
		d.Generation = d.CurrentGeneration
	}
}
