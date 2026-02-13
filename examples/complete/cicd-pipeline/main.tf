# =============================================================================
# CI/CD Pipeline Integration
# =============================================================================
#
# Plug Braintrust into your deployment pipeline. This example sets up:
#
#   - Service tokens with minimal scope for CI runners.
#   - Standardized scores and tags across all projects so eval results
#     are consistent and comparable.
#   - Webhook automations that fire when scores drop below thresholds.
#   - A monitoring service token for read-only dashboarding.
#
# Typical flow:
#
#   1. Developer pushes code → CI pipeline triggers.
#   2. CI runner uses a scoped service token to create an experiment
#      and push eval results.
#   3. If scores drop, a webhook fires to Slack / PagerDuty.
#   4. A monitoring dashboard reads results with a read-only token.
#
# =============================================================================

terraform {
  required_providers {
    braintrust = {
      source  = "braintrustdata/braintrust"
      version = "~> 0.1"
    }
  }
}

provider "braintrust" {}

# =============================================================================
# Variables
# =============================================================================

variable "slack_webhook_url" {
  description = "Slack incoming webhook URL for score regression alerts"
  type        = string
  default     = "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
}

variable "openai_api_key" {
  type      = string
  sensitive = true
}

variable "project_names" {
  description = "List of AI products to create standardized projects for"
  type        = list(string)
  default     = ["Search", "Chat", "Recommendations"]
}

# =============================================================================
# 1. Projects — one per AI product.
# =============================================================================

resource "braintrust_project" "products" {
  for_each    = toset(var.project_names)
  name        = each.value
  description = "${each.value} AI product evaluation"
}

# =============================================================================
# 2. Standardized scores — applied to EVERY project.
#
#    This ensures all teams measure the same things, making cross-project
#    comparisons meaningful.
# =============================================================================

resource "braintrust_project_score" "accuracy" {
  for_each    = braintrust_project.products
  project_id  = each.value.id
  name        = "Accuracy"
  score_type  = "slider"
  description = "Factual correctness of the response (0–1)"
}

resource "braintrust_project_score" "latency" {
  for_each    = braintrust_project.products
  project_id  = each.value.id
  name        = "Latency"
  score_type  = "slider"
  description = "Response latency normalized to 0–1 (lower is better)"
}

resource "braintrust_project_score" "safety" {
  for_each    = braintrust_project.products
  project_id  = each.value.id
  name        = "Safety"
  score_type  = "categorical"
  description = "Content safety classification"
}

# =============================================================================
# 3. Standardized tags — consistent across all projects.
# =============================================================================

resource "braintrust_project_tag" "ci" {
  for_each    = braintrust_project.products
  project_id  = each.value.id
  name        = "ci"
  description = "Automated CI pipeline run"
}

resource "braintrust_project_tag" "nightly" {
  for_each    = braintrust_project.products
  project_id  = each.value.id
  name        = "nightly"
  description = "Nightly regression run"
}

resource "braintrust_project_tag" "release" {
  for_each    = braintrust_project.products
  project_id  = each.value.id
  name        = "release-candidate"
  description = "Release candidate evaluation"
}

resource "braintrust_project_tag" "baseline" {
  for_each    = braintrust_project.products
  project_id  = each.value.id
  name        = "baseline"
  description = "Baseline comparison run"
}

# =============================================================================
# 4. Golden datasets — one per project for regression testing.
# =============================================================================

resource "braintrust_dataset" "golden" {
  for_each    = braintrust_project.products
  project_id  = each.value.id
  name        = "Golden Test Cases"
  description = "Curated regression test cases for ${each.key}"
}

# =============================================================================
# 5. Service tokens — scoped to specific tasks.
#
#    Principle of least privilege: each token can only do what it needs.
# =============================================================================

# Create service accounts for CI and monitoring.
resource "braintrust_service_account" "ci_runner" {
  name = "ci-runner"
}

resource "braintrust_service_account" "monitoring" {
  name = "monitoring-readonly"
}

# CI runner token: create experiments and push results. Cannot delete.
resource "braintrust_service_token" "ci_runner" {
  name               = "ci-runner"
  service_account_id = braintrust_service_account.ci_runner.id
}

# Monitoring token: read-only, used by dashboards and alerting.
resource "braintrust_service_token" "monitoring" {
  name               = "monitoring-readonly"
  service_account_id = braintrust_service_account.monitoring.id
}

# =============================================================================
# 6. Webhook automations — alert when scores regress.
# =============================================================================

resource "braintrust_project_automation" "accuracy_alert" {
  for_each    = braintrust_project.products
  project_id  = each.value.id
  name        = "Accuracy Regression Alert"
  description = "Fire when accuracy drops below 0.8 in CI runs"

  logs_config = {
    btql_filter      = "scores.accuracy < 0.8"
    interval_seconds = 300

    webhook_action = {
      url = var.slack_webhook_url
    }
  }
}

resource "braintrust_project_automation" "safety_alert" {
  for_each    = braintrust_project.products
  project_id  = each.value.id
  name        = "Safety Violation Alert"
  description = "Immediate alert on any unsafe content detection"

  logs_config = {
    btql_filter      = "scores.safety < 0.5"
    interval_seconds = 60

    webhook_action = {
      url = var.slack_webhook_url
    }
  }
}

# =============================================================================
# 7. Data retention — keep CI logs for 60 days, clean up automatically.
# =============================================================================

resource "braintrust_project_automation" "log_retention" {
  for_each   = braintrust_project.products
  project_id = each.value.id
  name       = "Log Retention 60d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 60
  }
}

# =============================================================================
# 8. AI provider credentials.
# =============================================================================

resource "braintrust_ai_secret" "openai" {
  name   = "OpenAI"
  type   = "openai"
  secret = var.openai_api_key
}

# =============================================================================
# Outputs — wire these into your CI config.
# =============================================================================

output "ci_runner_token" {
  description = "Service token for CI pipeline (store as CI secret)"
  value       = braintrust_service_token.ci_runner.key
  sensitive   = true
}

output "monitoring_token" {
  description = "Read-only token for monitoring dashboards"
  value       = braintrust_service_token.monitoring.key
  sensitive   = true
}

output "project_ids" {
  description = "Map of product name → project ID (use in CI scripts)"
  value       = { for k, v in braintrust_project.products : k => v.id }
}
