package provider

import (
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type camundaClusterMonitoringData struct {
	Id            types.String `tfsdk:"id"`
	ClusterID     types.String `tfsdk:"cluster_id"`
	MetricsTarget types.String `tfsdk:"metrics_target"`
	MetricsScheme types.String `tfsdk:"metrics_scheme"`
	MetricsPath   types.String `tfsdk:"metrics_path"`
	Conditions    types.List   `tfsdk:"conditions"`
}

func NewCamundaClusterMonitoringResource() resource.Resource {
	return &managedResource[camundaClusterMonitoringData]{
		typeName: "_cluster_monitoring",
		noun:     "cluster monitoring",
		schema:   clusterMonitoringSchema,
		describe: func(d camundaClusterMonitoringData) string { return "cluster " + d.ClusterID.ValueString() },
		create: func(op *op, plan camundaClusterMonitoringData) (camundaClusterMonitoringData, error) {
			clusterID := plan.ClusterID.ValueString()
			if err := op.client.WaitClusterHealthy(op.ctx, clusterID); err != nil {
				return plan, err
			}
			if err := op.client.ActivateMonitoring(op.ctx, clusterID); err != nil {
				return plan, err
			}

			// The plan is saved before waiting for the metrics endpoint, so
			// nothing may be unknown; the read after the wait fills these in.
			plan.Id = plan.ClusterID
			plan.MetricsTarget = types.StringNull()
			plan.MetricsScheme = types.StringNull()
			plan.MetricsPath = types.StringNull()
			plan.Conditions = types.ListNull(types.StringType)
			return plan, nil
		},
		// The Console sets up the metrics endpoint in the background.
		awaitReady: func(op *op, created camundaClusterMonitoringData) error {
			return op.client.WaitMonitoringActive(op.ctx, created.ClusterID.ValueString())
		},
		read: func(op *op, prior camundaClusterMonitoringData) (camundaClusterMonitoringData, error) {
			// An import only sets the id, which is the cluster ID; create has
			// set cluster_id.
			imported := prior.ClusterID.IsNull()
			prior.ClusterID = prior.Id

			found, err := op.client.GetMonitoring(op.ctx, prior.Id.ValueString())
			if errors.Is(err, errNotFound) && !imported && prior.MetricsTarget.IsNull() {
				// Activation was started but never seen finishing, for example
				// because Terraform was interrupted while waiting for the
				// metrics endpoint. While the Console is still setting it up, it
				// reports nothing, so don't forget the activation: the next
				// apply would activate it again and conflict. A cluster that is
				// gone is not worth keeping it for.
				if _, clusterErr := op.client.GetCluster(op.ctx, prior.Id.ValueString()); clusterErr != nil {
					return prior, clusterErr
				}
				return prior, nil
			}
			if err != nil {
				return prior, err
			}
			prior.MetricsTarget = nullIfEmpty(found.Target)
			prior.MetricsScheme = nullIfEmpty(found.Scheme)
			prior.MetricsPath = nullIfEmpty(found.Path)
			prior.Conditions = stringList(found.Conditions)
			return prior, nil
		},
		delete: func(op *op, prior camundaClusterMonitoringData) error {
			return op.client.DeactivateMonitoring(op.ctx, prior.ClusterID.ValueString())
		},
		importID: func(clusterID string) (camundaClusterMonitoringData, error) {
			return camundaClusterMonitoringData{Id: types.StringValue(clusterID), Conditions: types.ListNull(types.StringType)}, nil
		},
	}
}

func clusterMonitoringSchema() schema.Schema {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: description}
	}

	return schema.Schema{
		MarkdownDescription: "Manage external monitoring (bring your own monitoring) of a cluster on Camunda SaaS: " +
			"it exposes the cluster's metrics so an external system can scrape them. " +
			"Creating the resource activates it and waits until the metrics endpoint exists; destroying it deactivates it. " +
			"Use `camunda_cluster_monitoring_client` to create the credentials for scraping. " +
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
			"metrics_target": computed("Host to scrape the cluster's metrics from."),
			"metrics_scheme": computed("Scheme (`http` or `https`) to scrape the metrics with."),
			"metrics_path":   computed("Path to scrape the metrics from."),
			"conditions": schema.ListAttribute{
				MarkdownDescription: "Status conditions of the external monitoring, as reported by the Console. Each is a JSON document, so use `jsondecode` to read it.",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}
