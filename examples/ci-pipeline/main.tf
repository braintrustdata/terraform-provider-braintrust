# =============================================================================
# CI Pipeline
# =============================================================================
#
# Braintrust setup for running evals in CI:
#
#   - One project per environment (dev, staging, prod) with the same scores,
#     tags, and golden dataset, so results are comparable across environments.
#   - A CI writer service account and token for dev and staging. Each can
#     only write to its own project, and CI cannot write to prod at all.
#   - One read-only token for all environments, for CI jobs that only report
#     results (PR comments, release checks).
#   - Log retention per environment, and an alert when live prod traffic
#     scores poorly.
#
# Typical flow:
#
#   1. A push triggers CI, which runs evals in dev or staging with that
#      environment's token.
#   2. Each run is recorded as an experiment in the environment's project,
#      tagged "ci". The CI job fails the build if scores regress.
#   3. Once deployed, production logs are scored online, and an automation
#      posts to a webhook (Slack, PagerDuty) if accuracy drops.
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

# Most resources use BRAINTRUST_API_KEY from the environment.
provider "braintrust" {}

# Creating service tokens requires an organization-owner service token.
provider "braintrust" {
  alias   = "owner"
  api_key = var.owner_service_token
}

# =============================================================================
# Variables
# =============================================================================

variable "owner_service_token" {
  description = "Organization-owner service token, used only to create CI service tokens"
  type        = string
  sensitive   = true
}

variable "alert_webhook_url" {
  description = "Webhook that receives production accuracy alerts"
  type        = string
}

variable "product_name" {
  description = "Name of the AI product under test"
  type        = string
  default     = "Support Bot"
}

locals {
  # Per-environment log retention. Prod keeps logs longest.
  environments = {
    dev     = { retention_days = 14 }
    staging = { retention_days = 30 }
    prod    = { retention_days = 180 }
  }

  # Environments CI can write to. Prod is read-only for CI.
  writer_environments = toset(["dev", "staging"])
}

# =============================================================================
# 1. Projects, one per environment
# =============================================================================

resource "braintrust_project" "env" {
  for_each    = local.environments
  name        = "${var.product_name} (${each.key})"
  description = "${var.product_name} evals for the ${each.key} environment"
}

# =============================================================================
# 2. Standard scores, tags, and golden dataset in every environment
# =============================================================================

resource "braintrust_project_score" "accuracy" {
  for_each    = braintrust_project.env
  project_id  = each.value.id
  name        = "Accuracy"
  score_type  = "slider"
  description = "Factual correctness of the response (0-1)"
}

resource "braintrust_project_score" "safety" {
  for_each    = braintrust_project.env
  project_id  = each.value.id
  name        = "Safety"
  score_type  = "categorical"
  description = "Content safety classification"

  categories_json = jsonencode([
    { name = "Unsafe", value = 0 },
    { name = "Safe", value = 1 },
  ])
}

resource "braintrust_project_tag" "ci" {
  for_each    = braintrust_project.env
  project_id  = each.value.id
  name        = "ci"
  description = "Run triggered by CI"
  color       = "#2196F3"
}

resource "braintrust_project_tag" "release_candidate" {
  for_each    = braintrust_project.env
  project_id  = each.value.id
  name        = "release-candidate"
  description = "Evaluation of a release candidate"
  color       = "#4CAF50"
}

resource "braintrust_dataset" "golden" {
  for_each    = braintrust_project.env
  project_id  = each.value.id
  name        = "Golden Test Cases"
  description = "Curated regression cases evaluated on every CI run"
}

# =============================================================================
# 3. CI identities: writers for dev and staging, one reader for everything
# =============================================================================

resource "braintrust_service_account" "ci" {
  for_each = local.writer_environments
  name     = "ci-${each.key}"
}

resource "braintrust_service_token" "ci" {
  provider           = braintrust.owner
  for_each           = braintrust_service_account.ci
  name               = "ci-${each.key}"
  service_account_id = each.value.id
}

# What CI needs: push prompts, load dataset rows, and record eval results.
# It cannot delete anything or change access.
resource "braintrust_role" "ci_writer" {
  name        = "CI Writer"
  description = "Push prompts, load dataset rows, and write experiments and logs."

  member_permissions = [
    { permission = "read" },
    { permission = "create", restrict_object_type = "prompt" },
    { permission = "update", restrict_object_type = "prompt" },
    { permission = "update", restrict_object_type = "dataset" },
    { permission = "create", restrict_object_type = "experiment" },
    { permission = "update", restrict_object_type = "experiment" },
    { permission = "create", restrict_object_type = "project_log" },
    { permission = "update", restrict_object_type = "project_log" },
  ]
}

# Each writer gets the role on its own environment's project only.
resource "braintrust_acl" "ci" {
  for_each    = local.writer_environments
  object_type = "project"
  object_id   = braintrust_project.env[each.key].id
  user_id     = braintrust_service_account.ci[each.key].id
  role_id     = braintrust_role.ci_writer.id
}

# A read-only identity for CI jobs that report results without writing.
# Built-in roles like Viewer are looked up by name instead of defined.
data "braintrust_role" "viewer" {
  name = "Viewer"
}

resource "braintrust_service_account" "ci_reader" {
  name = "ci-reader"
}

resource "braintrust_service_token" "ci_reader" {
  provider           = braintrust.owner
  name               = "ci-reader"
  service_account_id = braintrust_service_account.ci_reader.id
}

resource "braintrust_acl" "ci_reader" {
  for_each    = braintrust_project.env
  object_type = "project"
  object_id   = each.value.id
  user_id     = braintrust_service_account.ci_reader.id
  role_id     = data.braintrust_role.viewer.id
}

# =============================================================================
# 4. Production alert and retention
# =============================================================================

# Log alerts watch project logs, not experiments, so they cannot catch a CI
# regression; gate that in the CI job. This one watches live prod traffic,
# assuming an online scoring rule writes the Accuracy score to prod logs.
resource "braintrust_project_automation" "prod_accuracy_alert" {
  project_id  = braintrust_project.env["prod"].id
  name        = "Low accuracy in production"
  description = "Fire when production traces score below 0.85 on accuracy"

  logs_config = {
    btql_filter      = "scores.Accuracy < 0.85"
    interval_seconds = 300

    webhook_action = {
      url = var.alert_webhook_url
    }
  }
}

resource "braintrust_project_automation" "log_retention" {
  for_each   = braintrust_project.env
  project_id = each.value.id
  name       = "Log retention ${local.environments[each.key].retention_days}d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = local.environments[each.key].retention_days
  }
}

# =============================================================================
# Outputs: store these as CI secrets and variables
# =============================================================================

output "project_ids" {
  description = "Environment name to project ID"
  value       = { for k, p in braintrust_project.env : k => p.id }
}

output "ci_tokens" {
  description = "Environment name to CI writer token (dev and staging)"
  value       = { for k, t in braintrust_service_token.ci : k => t.key }
  sensitive   = true
}

output "ci_reader_token" {
  description = "Read-only reporting token for all environments"
  value       = braintrust_service_token.ci_reader.key
  sensitive   = true
}
