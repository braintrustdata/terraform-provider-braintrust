package provider

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------------------------------------------------------------------------
// Terraform data models
// ---------------------------------------------------------------------------

// projectAutomationModel is the top-level Terraform model.
type projectAutomationModel struct {
	ID                      types.String                  `tfsdk:"id"`
	ProjectID               types.String                  `tfsdk:"project_id"`
	Name                    types.String                  `tfsdk:"name"`
	Description             types.String                  `tfsdk:"description"`
	UserID                  types.String                  `tfsdk:"user_id"`
	Created                 types.String                  `tfsdk:"created"`
	ConfigJSON              types.String                  `tfsdk:"config_json"`
	LogsConfig              *logsConfigModel              `tfsdk:"logs_config"`
	ExportConfig            *exportConfigModel            `tfsdk:"export_config"`
	RetentionConfig         *retentionConfigModel         `tfsdk:"retention_config"`
	EnvironmentUpdateConfig *environmentUpdateConfigModel `tfsdk:"environment_update_config"`
}

type logsConfigModel struct {
	BtqlFilter      types.String        `tfsdk:"btql_filter"`
	IntervalSeconds types.Int64         `tfsdk:"interval_seconds"`
	WebhookAction   *webhookActionModel `tfsdk:"webhook_action"`
	SlackAction     *slackActionModel   `tfsdk:"slack_action"`
}

type exportConfigModel struct {
	ExportType        types.String            `tfsdk:"export_type"`
	BtqlQuery         types.String            `tfsdk:"btql_query"`
	ExportPath        types.String            `tfsdk:"export_path"`
	Format            types.String            `tfsdk:"format"`
	IntervalSeconds   types.Int64             `tfsdk:"interval_seconds"`
	BatchSize         types.Int64             `tfsdk:"batch_size"`
	AWSIAMCredentials *awsIAMCredentialsModel `tfsdk:"aws_iam_credentials"`
}

type retentionConfigModel struct {
	ObjectType    types.String `tfsdk:"object_type"`
	RetentionDays types.Int64  `tfsdk:"retention_days"`
}

type environmentUpdateConfigModel struct {
	EnvironmentFilter types.List          `tfsdk:"environment_filter"`
	WebhookAction     *webhookActionModel `tfsdk:"webhook_action"`
	SlackAction       *slackActionModel   `tfsdk:"slack_action"`
}

type webhookActionModel struct {
	URL types.String `tfsdk:"url"`
}

type slackActionModel struct {
	WorkspaceID     types.String `tfsdk:"workspace_id"`
	Channel         types.String `tfsdk:"channel"`
	MessageTemplate types.String `tfsdk:"message_template"`
}

type awsIAMCredentialsModel struct {
	RoleARN    types.String `tfsdk:"role_arn"`
	ExternalID types.String `tfsdk:"external_id"`
}

// ---------------------------------------------------------------------------
// Model → JSON (for create / update requests)
// ---------------------------------------------------------------------------

// buildConfigJSON converts the Terraform model's config block into the JSON
// payload expected by the Braintrust API.
func buildConfigJSON(model *projectAutomationModel) (json.RawMessage, error) {
	count := 0
	for _, present := range []bool{model.LogsConfig != nil, model.ExportConfig != nil, model.RetentionConfig != nil, model.EnvironmentUpdateConfig != nil, !model.ConfigJSON.IsNull() && !model.ConfigJSON.IsUnknown()} {
		if present {
			count++
		}
	}
	if count != 1 {
		return nil, fmt.Errorf("exactly one of logs_config, export_config, retention_config, environment_update_config, or config_json must be specified")
	}
	if !model.ConfigJSON.IsNull() && !model.ConfigJSON.IsUnknown() {
		var config map[string]json.RawMessage
		if err := json.Unmarshal([]byte(model.ConfigJSON.ValueString()), &config); err != nil {
			return nil, fmt.Errorf("config_json must be a JSON object: %w", err)
		}
		var eventType string
		if err := json.Unmarshal(config["event_type"], &eventType); err != nil || eventType == "" {
			return nil, fmt.Errorf("config_json must contain a non-empty string event_type")
		}
		return json.Marshal(config)
	}
	if model.LogsConfig != nil {
		return buildLogsConfigJSON(model.LogsConfig)
	}
	if model.ExportConfig != nil {
		return buildExportConfigJSON(model.ExportConfig)
	}
	if model.RetentionConfig != nil {
		return buildRetentionConfigJSON(model.RetentionConfig)
	}
	if model.EnvironmentUpdateConfig != nil {
		return buildEnvironmentUpdateConfigJSON(model.EnvironmentUpdateConfig)
	}
	return nil, fmt.Errorf("no config block specified")
}

func buildLogsConfigJSON(m *logsConfigModel) (json.RawMessage, error) {
	action, err := buildActionJSON(m.WebhookAction, m.SlackAction)
	if err != nil {
		return nil, err
	}
	cfg := map[string]interface{}{
		"event_type":       "logs",
		"btql_filter":      m.BtqlFilter.ValueString(),
		"interval_seconds": m.IntervalSeconds.ValueInt64(),
		"action":           action,
	}
	return json.Marshal(cfg)
}

func buildExportConfigJSON(m *exportConfigModel) (json.RawMessage, error) {
	if m.AWSIAMCredentials == nil || m.AWSIAMCredentials.ExternalID.IsNull() || m.AWSIAMCredentials.ExternalID.IsUnknown() {
		return nil, fmt.Errorf("export_config requires aws_iam_credentials with role_arn and external_id")
	}
	if m.ExportType.ValueString() == "btql_query" && (m.BtqlQuery.IsNull() || m.BtqlQuery.IsUnknown() || m.BtqlQuery.ValueString() == "") {
		return nil, fmt.Errorf("export_config with export_type btql_query requires btql_query")
	}
	cfg := map[string]interface{}{
		"event_type":       "btql_export",
		"export_path":      m.ExportPath.ValueString(),
		"format":           m.Format.ValueString(),
		"interval_seconds": m.IntervalSeconds.ValueInt64(),
	}

	// Build export_definition.
	exportType := m.ExportType.ValueString()
	switch exportType {
	case "log_traces", "log_spans":
		cfg["export_definition"] = map[string]interface{}{"type": exportType}
	case "btql_query":
		cfg["export_definition"] = map[string]interface{}{
			"type":       "btql_query",
			"btql_query": m.BtqlQuery.ValueString(),
		}
	default:
		return nil, fmt.Errorf("unknown export_type: %s", exportType)
	}

	if !m.BatchSize.IsNull() && !m.BatchSize.IsUnknown() {
		cfg["batch_size"] = m.BatchSize.ValueInt64()
	}

	cfg["credentials"] = map[string]interface{}{
		"type":        "aws_iam",
		"role_arn":    m.AWSIAMCredentials.RoleARN.ValueString(),
		"external_id": m.AWSIAMCredentials.ExternalID.ValueString(),
	}

	return json.Marshal(cfg)
}

func buildRetentionConfigJSON(m *retentionConfigModel) (json.RawMessage, error) {
	cfg := map[string]interface{}{
		"event_type":     "retention",
		"object_type":    m.ObjectType.ValueString(),
		"retention_days": m.RetentionDays.ValueInt64(),
	}
	return json.Marshal(cfg)
}

func buildEnvironmentUpdateConfigJSON(m *environmentUpdateConfigModel) (json.RawMessage, error) {
	action, err := buildActionJSON(m.WebhookAction, m.SlackAction)
	if err != nil {
		return nil, err
	}
	cfg := map[string]interface{}{
		"event_type": "environment_update",
		"action":     action,
	}

	if !m.EnvironmentFilter.IsNull() && !m.EnvironmentFilter.IsUnknown() {
		filters := stringListFromModel(m.EnvironmentFilter)
		if len(filters) > 0 {
			cfg["environment_filter"] = filters
		}
	}

	return json.Marshal(cfg)
}

func buildActionJSON(webhook *webhookActionModel, slack *slackActionModel) (map[string]interface{}, error) {
	if (webhook == nil) == (slack == nil) {
		return nil, fmt.Errorf("exactly one of webhook_action or slack_action must be specified")
	}
	if webhook != nil {
		return map[string]interface{}{
			"type": "webhook",
			"url":  webhook.URL.ValueString(),
		}, nil
	}
	if slack != nil {
		action := map[string]interface{}{
			"type":         "slack",
			"workspace_id": slack.WorkspaceID.ValueString(),
			"channel":      slack.Channel.ValueString(),
		}
		if !slack.MessageTemplate.IsNull() && !slack.MessageTemplate.IsUnknown() {
			action["message_template"] = slack.MessageTemplate.ValueString()
		}
		return action, nil
	}
	return nil, fmt.Errorf("no action specified (webhook_action or slack_action)")
}

// ---------------------------------------------------------------------------
// JSON → Model (for read / refresh)
// ---------------------------------------------------------------------------

// parseConfigToModel populates the appropriate config block in the Terraform
// model from the raw JSON returned by the API.
func parseConfigToModel(raw json.RawMessage, model *projectAutomationModel) error {
	// Determine the config variant.
	var disc struct {
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(raw, &disc); err != nil {
		return fmt.Errorf("failed to parse config event_type: %w", err)
	}

	// Reset all config blocks.
	model.LogsConfig = nil
	model.ExportConfig = nil
	model.RetentionConfig = nil
	model.EnvironmentUpdateConfig = nil
	// Keep the raw config form chosen by the user, even for a known event type.
	// Unknown event types use this form on import so their full payload survives.
	if (!model.ConfigJSON.IsNull() && !model.ConfigJSON.IsUnknown()) ||
		(disc.EventType != "logs" && disc.EventType != "btql_export" && disc.EventType != "retention" && disc.EventType != "environment_update") {
		var config map[string]json.RawMessage
		if err := json.Unmarshal(raw, &config); err != nil || config == nil || disc.EventType == "" {
			return fmt.Errorf("automation config must be a JSON object with event_type")
		}
		model.ConfigJSON = promptJSONState(raw, model.ConfigJSON)
		return nil
	}
	model.ConfigJSON = types.StringNull()

	switch disc.EventType {
	case "logs":
		return parseLogsConfig(raw, model)
	case "btql_export":
		return parseExportConfig(raw, model)
	case "retention":
		return parseRetentionConfig(raw, model)
	case "environment_update":
		return parseEnvironmentUpdateConfig(raw, model)
	default:
		return fmt.Errorf("unknown config event_type: %q", disc.EventType)
	}
}

func parseLogsConfig(raw json.RawMessage, model *projectAutomationModel) error {
	var cfg struct {
		BtqlFilter      string          `json:"btql_filter"`
		IntervalSeconds int64           `json:"interval_seconds"`
		Action          json.RawMessage `json:"action"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	webhook, slack, err := parseAction(cfg.Action)
	if err != nil {
		return err
	}

	model.LogsConfig = &logsConfigModel{
		BtqlFilter:      types.StringValue(cfg.BtqlFilter),
		IntervalSeconds: types.Int64Value(cfg.IntervalSeconds),
		WebhookAction:   webhook,
		SlackAction:     slack,
	}
	return nil
}

func parseExportConfig(raw json.RawMessage, model *projectAutomationModel) error {
	var cfg struct {
		ExportDefinition json.RawMessage `json:"export_definition"`
		ExportPath       string          `json:"export_path"`
		Format           string          `json:"format"`
		IntervalSeconds  int64           `json:"interval_seconds"`
		BatchSize        *int64          `json:"batch_size,omitempty"`
		Credentials      json.RawMessage `json:"credentials,omitempty"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	// Parse export_definition.
	var exportDef struct {
		Type      string  `json:"type"`
		BtqlQuery *string `json:"btql_query,omitempty"`
	}
	if err := json.Unmarshal(cfg.ExportDefinition, &exportDef); err != nil {
		return fmt.Errorf("failed to parse export_definition: %w", err)
	}

	m := &exportConfigModel{
		ExportType:      types.StringValue(exportDef.Type),
		ExportPath:      types.StringValue(cfg.ExportPath),
		Format:          types.StringValue(cfg.Format),
		IntervalSeconds: types.Int64Value(cfg.IntervalSeconds),
		BtqlQuery:       types.StringNull(),
		BatchSize:       types.Int64Null(),
	}

	if exportDef.BtqlQuery != nil {
		m.BtqlQuery = types.StringValue(*exportDef.BtqlQuery)
	}
	if cfg.BatchSize != nil {
		m.BatchSize = types.Int64Value(*cfg.BatchSize)
	}

	// Parse credentials.
	if len(cfg.Credentials) > 0 && string(cfg.Credentials) != "null" {
		var creds struct {
			Type       string  `json:"type"`
			RoleARN    string  `json:"role_arn"`
			ExternalID *string `json:"external_id,omitempty"`
		}
		if err := json.Unmarshal(cfg.Credentials, &creds); err != nil {
			return fmt.Errorf("failed to parse credentials: %w", err)
		}
		m.AWSIAMCredentials = &awsIAMCredentialsModel{
			RoleARN:    types.StringValue(creds.RoleARN),
			ExternalID: stringPtrToTFValue(creds.ExternalID),
		}
	}

	model.ExportConfig = m
	return nil
}

func parseRetentionConfig(raw json.RawMessage, model *projectAutomationModel) error {
	var cfg struct {
		ObjectType    string `json:"object_type"`
		RetentionDays int64  `json:"retention_days"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	model.RetentionConfig = &retentionConfigModel{
		ObjectType:    types.StringValue(cfg.ObjectType),
		RetentionDays: types.Int64Value(cfg.RetentionDays),
	}
	return nil
}

func parseEnvironmentUpdateConfig(raw json.RawMessage, model *projectAutomationModel) error {
	var cfg struct {
		EnvironmentFilter []string        `json:"environment_filter,omitempty"`
		Action            json.RawMessage `json:"action"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	webhook, slack, err := parseAction(cfg.Action)
	if err != nil {
		return err
	}

	model.EnvironmentUpdateConfig = &environmentUpdateConfigModel{
		EnvironmentFilter: stringSliceToTFList(cfg.EnvironmentFilter),
		WebhookAction:     webhook,
		SlackAction:       slack,
	}
	return nil
}

func parseAction(raw json.RawMessage) (*webhookActionModel, *slackActionModel, error) {
	var disc struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &disc); err != nil {
		return nil, nil, fmt.Errorf("failed to parse action type: %w", err)
	}

	switch disc.Type {
	case "webhook":
		var a struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, nil, err
		}
		return &webhookActionModel{URL: types.StringValue(a.URL)}, nil, nil

	case "slack":
		var a struct {
			WorkspaceID     string  `json:"workspace_id"`
			Channel         string  `json:"channel"`
			MessageTemplate *string `json:"message_template,omitempty"`
		}
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, nil, err
		}
		return nil, &slackActionModel{
			WorkspaceID:     types.StringValue(a.WorkspaceID),
			Channel:         types.StringValue(a.Channel),
			MessageTemplate: stringPtrToTFValue(a.MessageTemplate),
		}, nil

	default:
		return nil, nil, fmt.Errorf("unknown action type: %q", disc.Type)
	}
}
