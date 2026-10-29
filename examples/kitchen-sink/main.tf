# =============================================================================
# Kitchen Sink
# =============================================================================
#
# Every resource and data source in the provider, wired together around one
# project. Use it as a reference for how the pieces connect; see the other
# examples in this directory for focused setups.
#
#   export BRAINTRUST_API_KEY="..."
#   terraform apply -var="my_email=you@company.com" -var="openai_api_key=..." \
#     -var="owner_service_token=..."
#
# =============================================================================

terraform {
  required_providers {
    braintrust = {
      source = "braintrustdata/braintrust"
      # version = "x.y.z" # pin a specific version
    }
  }
}

provider "braintrust" {}

# Creating service tokens requires an organization-owner service token.
provider "braintrust" {
  alias   = "owner"
  api_key = var.owner_service_token
}

variable "my_email" {
  description = "Your email; you must already be a member of the org"
  type        = string
}

variable "openai_api_key" {
  type      = string
  sensitive = true
}

variable "owner_service_token" {
  description = "Organization-owner service token, used only to create the service token"
  type        = string
  sensitive   = true
}

variable "alert_webhook_url" {
  type    = string
  default = "https://hooks.example.com/braintrust-alerts"
}

variable "org_experiment_retention_days" {
  description = "Org-wide experiment retention. Requires a plan with custom retention; leave null to skip."
  type        = number
  default     = null
}

# =============================================================================
# Project and its contents
# =============================================================================

resource "braintrust_project" "app" {
  name        = "kitchen-sink"
  description = "Every Braintrust Terraform resource in one place"
}

locals {
  org_id = braintrust_project.app.org_id
}

resource "braintrust_dataset" "golden" {
  project_id  = braintrust_project.app.id
  name        = "golden"
  description = "Curated evaluation cases"
  tags        = ["gold", "reviewed"]
  metadata_json = jsonencode({
    owner = "evaluation-team"
  })
}

resource "braintrust_project_score" "accuracy" {
  project_id  = braintrust_project.app.id
  name        = "Accuracy"
  score_type  = "slider"
  description = "Factual correctness (0-1)"
}

resource "braintrust_project_score" "tone" {
  project_id  = braintrust_project.app.id
  name        = "Tone"
  score_type  = "categorical"
  description = "Human review of response tone"

  categories_json = jsonencode([
    { name = "Off", value = 0 },
    { name = "Acceptable", value = 0.5 },
    { name = "Great", value = 1 },
  ])
  config_json = jsonencode({
    multi_select = false
    object_types = ["project_logs"]
  })
}

resource "braintrust_project_tag" "baseline" {
  project_id  = braintrust_project.app.id
  name        = "baseline"
  description = "Baseline runs to compare against"
  color       = "#4CAF50"
}

# A prompt and a code scorer. Both are braintrust_function.
resource "braintrust_function" "summarizer" {
  project_id = braintrust_project.app.id
  name       = "Summarizer"
  slug       = "summarizer"

  function_data_json = jsonencode({ type = "prompt" })
  prompt_data_json = jsonencode({
    prompt  = { type = "completion", content = "Summarize {{input}}" }
    options = { model = "gpt-4o-mini" }
  })
}

resource "braintrust_function" "non_empty" {
  project_id    = braintrust_project.app.id
  name          = "Non-empty output"
  slug          = "non-empty-output"
  function_type = "scorer"

  function_data_json = jsonencode({
    type = "code"
    data = {
      type            = "inline"
      runtime_context = { runtime = "node", version = "20" }
      code            = "function handler({ output }) { return output ? 1 : 0; }"
    }
  })
}

# A custom Topics facet: its preprocessor turns each trace into text, and the
# facet prompt summarizes that text. The topics automation below runs it.
resource "braintrust_function" "user_message" {
  project_id    = braintrust_project.app.id
  name          = "User message"
  slug          = "user-message"
  function_type = "preprocessor"

  function_data_json = jsonencode({
    type = "code"
    data = {
      type            = "inline"
      runtime_context = { runtime = "quickjs", version = "ES2023" }
      code            = "function handler(spanData) { return { text: String(spanData.input || '') }; }"
    }
  })
}

resource "braintrust_function" "product_area" {
  project_id    = braintrust_project.app.id
  name          = "Product area"
  slug          = "product-area"
  function_type = "facet"

  function_data_json = jsonencode({
    type             = "facet"
    prompt           = "Which product area is the user asking about? Answer in two to four words, or NONE if unclear."
    no_match_pattern = "^NONE$"
    preprocessor     = { type = "function", id = braintrust_function.user_message.id }
  })
}

resource "braintrust_mcp_server" "tools" {
  project_id  = braintrust_project.app.id
  name        = "Shared tools"
  url         = "https://tools.example.com/mcp"
  description = "Tools available to prompts in this project"
}

# A saved log view and a dashboard with a SQL chart. Both are braintrust_view.
# To manage a view with filters, build it in the UI and export it with
# braintrust-terraforming to get its view_data_json.
resource "braintrust_view" "all_logs" {
  object_type = "project"
  object_id   = braintrust_project.app.id
  view_type   = "logs"
  name        = "All logs"

  view_data_json = jsonencode({
    search = { filter = [] }
  })
}

resource "braintrust_view" "health" {
  object_type = "project"
  object_id   = braintrust_project.app.id
  view_type   = "monitor"
  name        = "Project health"

  options_json = jsonencode({
    viewType = "monitor"
    options = {
      type       = "project"
      projectId  = braintrust_project.app.id
      spanType   = "range"
      rangeValue = "7d"
    }
  })
  view_data_json = jsonencode({
    custom_charts = {
      version = "0.0.0"
      layout  = { type = "linear", order = ["spans"] }
      charts = {
        spans = {
          title    = "Total spans"
          chartSql = "SELECT count(1) AS spans FROM project_logs()"
          viz      = { type = "singleValue", unitType = "count" }
        }
      }
    }
  })
}

# =============================================================================
# Secrets and environment variables
# =============================================================================

# Org-wide AI provider credential.
resource "braintrust_ai_secret" "openai" {
  name   = "kitchen-sink-openai"
  type   = "openai"
  secret = var.openai_api_key
}

# A plain project environment variable. Set secret_type to make it a
# project-level AI provider credential instead.
resource "braintrust_env_var" "api_base_url" {
  object_type = "project"
  object_id   = braintrust_project.app.id
  name        = "API_BASE_URL"
  value       = "https://api.example.com"
}

# =============================================================================
# Automations
# =============================================================================

resource "braintrust_project_automation" "accuracy_alert" {
  project_id  = braintrust_project.app.id
  name        = "Low accuracy"
  description = "Fire when accuracy drops below 0.8"

  logs_config = {
    btql_filter      = "scores.Accuracy < 0.8"
    interval_seconds = 300

    webhook_action = {
      url = var.alert_webhook_url
    }
  }
}

resource "braintrust_project_automation" "log_retention" {
  project_id = braintrust_project.app.id
  name       = "Log retention 90d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 90
  }
}

# Topics has no structured block, so its config is raw JSON. Braintrust
# generates topic maps from logs; leave topic_map_functions empty.
resource "braintrust_project_automation" "topics" {
  project_id  = braintrust_project.app.id
  name        = "Topics"
  description = "Cluster a sample of traces by product area"

  config_json = jsonencode({
    event_type          = "topic"
    sampling_rate       = 0.25
    btql_filter         = "span_attributes.type = 'llm'"
    facet_functions     = [{ type = "function", id = braintrust_function.product_area.id }]
    topic_map_functions = []
    rerun_seconds       = 86400     # regenerate topic maps daily
    backfill_time_range = "259200s" # classify the last 3 days on creation
  })
}

resource "braintrust_org_automation" "experiment_retention" {
  count  = var.org_experiment_retention_days == null ? 0 : 1
  org_id = local.org_id
  name   = "Experiment retention"

  retention_config = {
    object_type    = "experiment"
    retention_days = var.org_experiment_retention_days
  }
}

# =============================================================================
# Access control
# =============================================================================

data "braintrust_user" "me" {
  email = var.my_email
}

data "braintrust_role" "viewer" {
  name = "Viewer"
}

# A custom role that extends the built-in Viewer role.
resource "braintrust_role" "dataset_editor" {
  name         = "Dataset Editor"
  description  = "Viewer, plus create and update datasets"
  member_roles = [data.braintrust_role.viewer.id]

  member_permissions = [
    { permission = "create", restrict_object_type = "dataset" },
    { permission = "update", restrict_object_type = "dataset" },
  ]
}

# Members can be listed inline (member_users) or managed one at a time with
# braintrust_group_membership. Use one style per group, not both.
resource "braintrust_group" "editors" {
  name        = "Kitchen sink editors"
  description = "Can edit datasets in the kitchen-sink project"
}

resource "braintrust_group_membership" "me" {
  group_id = braintrust_group.editors.id
  user_id  = data.braintrust_user.me.id
}

resource "braintrust_group" "readers" {
  name          = "Kitchen sink readers"
  member_groups = [braintrust_group.editors.id]
}

resource "braintrust_acl" "editors" {
  object_type = "project"
  object_id   = braintrust_project.app.id
  group_id    = braintrust_group.editors.id
  role_id     = braintrust_role.dataset_editor.id
}

# Project groups let one ACL cover many projects.
resource "braintrust_project_group" "examples" {
  org_id = local.org_id
  name   = "Examples"
}

resource "braintrust_project_group_membership" "app" {
  project_group_id = braintrust_project_group.examples.id
  project_id       = braintrust_project.app.id
}

# project_group_projects covers the group's projects; "project_group" would
# only cover the group object itself.
resource "braintrust_acl" "readers" {
  object_type = "project_group_projects"
  object_id   = braintrust_project_group.examples.id
  group_id    = braintrust_group.readers.id
  permission  = "read"
}

# A service account is a non-human org member. Grant it access with an ACL
# like any user, and give it a token for programmatic access.
resource "braintrust_service_account" "bot" {
  name = "kitchen-sink-bot"
}

resource "braintrust_service_token" "bot" {
  provider           = braintrust.owner
  name               = "kitchen-sink-bot"
  service_account_id = braintrust_service_account.bot.id
}

resource "braintrust_acl" "bot" {
  object_type          = "project"
  object_id            = braintrust_project.app.id
  user_id              = braintrust_service_account.bot.id
  permission           = "read"
  restrict_object_type = "dataset"
}

# =============================================================================
# Data sources: look up objects managed elsewhere. Here they read back the
# objects above, since this configuration has nothing else to point at.
# =============================================================================

data "braintrust_project" "app" {
  name = braintrust_project.app.name
}

data "braintrust_dataset" "golden" {
  project_id = data.braintrust_project.app.id
  name       = braintrust_dataset.golden.name
}

data "braintrust_function" "summarizer" {
  project_id = data.braintrust_project.app.id
  name       = braintrust_function.summarizer.name
}

data "braintrust_project_group" "examples" {
  org_id = local.org_id
  name   = braintrust_project_group.examples.name
}

data "braintrust_group" "editors" {
  name = braintrust_group.editors.name
}

# =============================================================================
# Outputs
# =============================================================================

output "project_id" {
  value = data.braintrust_project.app.id
}

output "lookups" {
  value = {
    dataset       = data.braintrust_dataset.golden.id
    function_slug = data.braintrust_function.summarizer.slug
    project_group = data.braintrust_project_group.examples.id
    editors_group = data.braintrust_group.editors.id
  }
}

output "bot_token" {
  value     = braintrust_service_token.bot.key
  sensitive = true
}
