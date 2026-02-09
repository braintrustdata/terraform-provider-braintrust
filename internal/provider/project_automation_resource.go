package provider

import (
	"context"
	"fmt"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
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

var (
	_ resource.Resource                = &projectAutomationResource{}
	_ resource.ResourceWithConfigure   = &projectAutomationResource{}
	_ resource.ResourceWithImportState = &projectAutomationResource{}
)

// NewProjectAutomationResource returns a new project automation resource.
func NewProjectAutomationResource() resource.Resource {
	return &projectAutomationResource{}
}

type projectAutomationResource struct {
	client *client.Client
}

func (r *projectAutomationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_automation"
}

// ---------------------------------------------------------------------------
// Schema helpers — reusable sub-schemas for webhook / slack action.
// ---------------------------------------------------------------------------

func webhookActionAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Send an HTTP POST to a webhook URL when the automation triggers.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Required:    true,
				Description: "The webhook URL to send the request to.",
			},
		},
	}
}

func slackActionAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Post a message to a Slack channel when the automation triggers.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": schema.StringAttribute{
				Required:    true,
				Description: "The Slack workspace ID.",
			},
			"channel": schema.StringAttribute{
				Required:    true,
				Description: "The Slack channel ID to post to.",
			},
			"message_template": schema.StringAttribute{
				Optional:    true,
				Description: "Custom message template for the alert.",
			},
		},
	}
}

func (r *projectAutomationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Braintrust project automation rule. Automations allow you to " +
			"set up alerts (webhook or Slack), scheduled data exports, data retention policies, " +
			"and environment-update notifications for a project.\n\n" +
			"Exactly one of `logs_config`, `export_config`, `retention_config`, or " +
			"`environment_update_config` must be specified.",
		Attributes: map[string]schema.Attribute{
			"id": computedStringAttribute("Unique identifier for the project automation."),
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "The project this automation belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the project automation.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Textual description of the project automation.",
			},
			"user_id": computedStringAttribute("The user who created this automation."),
			"created": computedStringAttribute("Date of creation."),

			// ---------------------------------------------------------------
			// Config variant 1: logs — alert on log events matching a filter.
			// ---------------------------------------------------------------
			"logs_config": schema.SingleNestedAttribute{
				Optional: true,
				Description: "Alert when log events match a BTQL filter. " +
					"Exactly one of webhook_action or slack_action must be specified.",
				Validators: []validator.Object{
					objectvalidator.ExactlyOneOf(
						path.MatchRoot("export_config"),
						path.MatchRoot("retention_config"),
						path.MatchRoot("environment_update_config"),
					),
				},
				Attributes: map[string]schema.Attribute{
					"btql_filter": schema.StringAttribute{
						Required:    true,
						Description: "BTQL filter expression to match log rows.",
					},
					"interval_seconds": schema.Int64Attribute{
						Required:    true,
						Description: "Fire the action at most once per this many seconds (1–2592000).",
						Validators: []validator.Int64{
							int64validator.Between(1, 2592000),
						},
					},
					"webhook_action": webhookActionAttribute(),
					"slack_action":   slackActionAttribute(),
				},
			},

			// ---------------------------------------------------------------
			// Config variant 2: btql_export — scheduled data export to S3.
			// ---------------------------------------------------------------
			"export_config": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Schedule periodic data exports to cloud storage (e.g. S3).",
				Attributes: map[string]schema.Attribute{
					"export_type": schema.StringAttribute{
						Required:    true,
						Description: "What to export: \"log_traces\", \"log_spans\", or \"btql_query\".",
						Validators: []validator.String{
							stringvalidator.OneOf("log_traces", "log_spans", "btql_query"),
						},
					},
					"btql_query": schema.StringAttribute{
						Optional:    true,
						Description: "The BTQL query to export. Required when export_type is \"btql_query\".",
					},
					"export_path": schema.StringAttribute{
						Required:    true,
						Description: "Storage path including protocol, e.g. \"s3://bucket/prefix\".",
					},
					"format": schema.StringAttribute{
						Required:    true,
						Description: "Export format: \"jsonl\" or \"parquet\".",
						Validators: []validator.String{
							stringvalidator.OneOf("jsonl", "parquet"),
						},
					},
					"interval_seconds": schema.Int64Attribute{
						Required:    true,
						Description: "Export interval in seconds (1–2592000).",
						Validators: []validator.Int64{
							int64validator.Between(1, 2592000),
						},
					},
					"batch_size": schema.Int64Attribute{
						Optional:    true,
						Description: "Number of rows to export in each batch.",
					},
					"aws_iam_credentials": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "AWS IAM role credentials for cross-account S3 access.",
						Attributes: map[string]schema.Attribute{
							"role_arn": schema.StringAttribute{
								Required:    true,
								Description: "The ARN of the IAM role to assume.",
							},
							"external_id": optionalComputedStringAttribute(
								"The external ID for the role trust policy. Auto-generated if omitted.",
							),
						},
					},
				},
			},

			// ---------------------------------------------------------------
			// Config variant 3: retention — data retention policy.
			// ---------------------------------------------------------------
			"retention_config": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Set a data retention policy for the project.",
				Attributes: map[string]schema.Attribute{
					"object_type": schema.StringAttribute{
						Required:    true,
						Description: "The type of data to apply retention to: \"project_logs\", \"experiment\", or \"dataset\".",
						Validators: []validator.String{
							stringvalidator.OneOf("project_logs", "experiment", "dataset"),
						},
					},
					"retention_days": schema.Int64Attribute{
						Required:    true,
						Description: "Number of days to retain data. 0 means delete immediately.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
				},
			},

			// ---------------------------------------------------------------
			// Config variant 4: environment_update — notify on env changes.
			// ---------------------------------------------------------------
			"environment_update_config": schema.SingleNestedAttribute{
				Optional: true,
				Description: "Notify when an environment is updated. " +
					"Exactly one of webhook_action or slack_action must be specified.",
				Attributes: map[string]schema.Attribute{
					"environment_filter": schema.ListAttribute{
						Optional:    true,
						ElementType: types.StringType,
						Description: "Optional list of environment slugs to filter by. If empty, all environments trigger the action.",
					},
					"webhook_action": webhookActionAttribute(),
					"slack_action":   slackActionAttribute(),
				},
			},
		},
	}
}

func (r *projectAutomationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

func (r *projectAutomationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectAutomationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	configJSON, err := buildConfigJSON(&plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Config", err.Error())
		return
	}

	createReq := client.CreateProjectAutomationRequest{
		ProjectID:   plan.ProjectID.ValueString(),
		Name:        plan.Name.ValueString(),
		Description: tfStringToPtr(plan.Description),
		Config:      configJSON,
	}

	tflog.Debug(ctx, "Creating project automation", map[string]any{
		"project_id": createReq.ProjectID,
		"name":       createReq.Name,
	})

	automation, err := r.client.CreateProjectAutomation(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Project Automation", err.Error())
		return
	}

	// Read back to ensure state is consistent.
	automation, err = r.client.GetProjectAutomation(ctx, automation.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Project Automation After Create", err.Error())
		return
	}

	mapProjectAutomationToModel(automation, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectAutomationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectAutomationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	automation, err := r.client.GetProjectAutomation(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Project automation not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Project Automation", err.Error())
		return
	}

	mapProjectAutomationToModel(automation, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *projectAutomationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state projectAutomationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	configJSON, err := buildConfigJSON(&plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Config", err.Error())
		return
	}

	name := plan.Name.ValueString()
	updateReq := client.UpdateProjectAutomationRequest{
		Name:        &name,
		Description: tfStringToPtr(plan.Description),
		Config:      configJSON,
	}

	tflog.Debug(ctx, "Updating project automation", map[string]any{
		"id":   state.ID.ValueString(),
		"name": name,
	})

	automation, err := r.client.UpdateProjectAutomation(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Project Automation", err.Error())
		return
	}

	mapProjectAutomationToModel(automation, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectAutomationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectAutomationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	tflog.Debug(ctx, "Deleting project automation", map[string]any{
		"id": state.ID.ValueString(),
	})

	err := r.client.DeleteProjectAutomation(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Error Deleting Project Automation", err.Error())
	}
}

func (r *projectAutomationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// Mapping: API response → Terraform model
// ---------------------------------------------------------------------------

func mapProjectAutomationToModel(a *client.ProjectAutomation, model *projectAutomationModel) {
	model.ID = types.StringValue(a.ID)
	model.ProjectID = types.StringValue(a.ProjectID)
	model.Name = types.StringValue(a.Name)
	model.Description = stringPtrToTFValue(a.Description)
	model.UserID = stringPtrToTFValue(a.UserID)
	model.Created = stringPtrToTFValue(a.Created)

	if len(a.Config) > 0 {
		// Errors here are logged but don't block — state will be partially
		// populated and a subsequent plan will detect drift.
		if err := parseConfigToModel(a.Config, model); err != nil {
			fmt.Printf("[WARN] failed to parse project_automation config: %v\n", err)
		}
	}
}
