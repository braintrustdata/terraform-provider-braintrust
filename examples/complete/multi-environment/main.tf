# =============================================================================
# Multi-Environment Promotion (Dev → Staging → Prod)
# =============================================================================
#
# Mirrors how software teams already think about environments:
#
#   Dev     — Engineers have full access. Move fast, break things.
#   Staging — Engineers can create experiments but not delete. QA reviews.
#   Prod    — Engineers read-only. Only SRE/ops can make changes.
#
# Each "environment" is a separate Braintrust project. This gives you:
#   - Clear isolation between environments.
#   - Different permission tiers per environment.
#   - Separate retention policies and alerting thresholds.
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

variable "product_name" {
  description = "Name of the AI product"
  type        = string
  default     = "AI Search"
}

variable "engineer_emails" {
  type    = list(string)
  default = ["eng1@company.com", "eng2@company.com"]
}

variable "sre_emails" {
  description = "SRE / ops team who manage production"
  type        = list(string)
  default     = ["sre@company.com"]
}

variable "qa_emails" {
  description = "QA engineers who review staging"
  type        = list(string)
  default     = ["qa@company.com"]
}

variable "lead_emails" {
  description = "Tech leads with full access everywhere"
  type        = list(string)
  default     = ["lead@company.com"]
}

variable "slack_webhook_url" {
  type    = string
  default = "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
}

variable "openai_api_key" {
  type      = string
  sensitive = true
}

# =============================================================================
# 1. Users
# =============================================================================

data "braintrust_user" "engineers" {
  for_each = toset(var.engineer_emails)
  email    = each.value
}

data "braintrust_user" "sre" {
  for_each = toset(var.sre_emails)
  email    = each.value
}

data "braintrust_user" "qa" {
  for_each = toset(var.qa_emails)
  email    = each.value
}

data "braintrust_user" "leads" {
  for_each = toset(var.lead_emails)
  email    = each.value
}

# =============================================================================
# 2. Environment projects.
# =============================================================================

resource "braintrust_project" "dev" {
  name        = "${var.product_name} — Dev"
  description = "Development environment: move fast, experiment freely"
}

resource "braintrust_project" "staging" {
  name        = "${var.product_name} — Staging"
  description = "Staging environment: pre-production validation"
}

resource "braintrust_project" "prod" {
  name        = "${var.product_name} — Prod"
  description = "Production environment: read-mostly, changes require SRE"
}

# =============================================================================
# 3. Roles — escalating restrictions per environment.
# =============================================================================

# Dev: full CRUD (the built-in Engineer role equivalent).
resource "braintrust_role" "dev_engineer" {
  name        = "Dev Engineer"
  description = "Full CRUD in dev environment. Move fast."

  member_permissions = [
    { permission = "create" },
    { permission = "read" },
    { permission = "update" },
    { permission = "delete" },
  ]
}

# Staging: create + read + update, but no delete.
resource "braintrust_role" "staging_engineer" {
  name        = "Staging Engineer"
  description = "Create and update in staging. No delete — preserve test history."

  member_permissions = [
    { permission = "read" },
    { permission = "create", restrict_object_type = "experiment" },
    { permission = "update", restrict_object_type = "experiment" },
    { permission = "create", restrict_object_type = "dataset" },
    { permission = "update", restrict_object_type = "dataset" },
    { permission = "create", restrict_object_type = "project_log" },
    { permission = "update", restrict_object_type = "project_log" },
  ]
}

# Prod: read-only for engineers. SRE uses the full-access role.
resource "braintrust_role" "prod_reader" {
  name        = "Prod Reader"
  description = "Read-only access to production environment."

  member_permissions = [
    { permission = "read" },
  ]
}

resource "braintrust_role" "prod_operator" {
  name        = "Prod Operator"
  description = "Full CRUD + ACL management in production. For SRE only."

  member_permissions = [
    { permission = "create" },
    { permission = "read" },
    { permission = "update" },
    { permission = "delete" },
    { permission = "create_acls" },
    { permission = "read_acls" },
    { permission = "update_acls" },
    { permission = "delete_acls" },
  ]
}

# =============================================================================
# 4. Groups
# =============================================================================

resource "braintrust_group" "engineers" {
  name         = "Engineers"
  description  = "AI/ML engineers"
  member_users = [for u in data.braintrust_user.engineers : u.id]
}

resource "braintrust_group" "sre" {
  name         = "SRE"
  description  = "Site reliability / ops team"
  member_users = [for u in data.braintrust_user.sre : u.id]
}

resource "braintrust_group" "qa" {
  name         = "QA"
  description  = "QA engineers"
  member_users = [for u in data.braintrust_user.qa : u.id]
}

resource "braintrust_group" "leads" {
  name         = "Tech Leads"
  description  = "Tech leads with full access to all environments"
  member_users = [for u in data.braintrust_user.leads : u.id]
}

# =============================================================================
# 5. ACLs — graduated access per environment.
# =============================================================================

# --- Dev: engineers get full access ---
resource "braintrust_acl" "eng_dev" {
  object_type = "project"
  object_id   = braintrust_project.dev.id
  group_id    = braintrust_group.engineers.id
  role_id     = braintrust_role.dev_engineer.id
}

# --- Staging: engineers can create/update, QA reviews ---
resource "braintrust_acl" "eng_staging" {
  object_type = "project"
  object_id   = braintrust_project.staging.id
  group_id    = braintrust_group.engineers.id
  role_id     = braintrust_role.staging_engineer.id
}

resource "braintrust_acl" "qa_staging" {
  object_type = "project"
  object_id   = braintrust_project.staging.id
  group_id    = braintrust_group.qa.id
  role_id     = braintrust_role.staging_engineer.id
}

# --- Prod: engineers read-only, SRE operates ---
resource "braintrust_acl" "eng_prod" {
  object_type = "project"
  object_id   = braintrust_project.prod.id
  group_id    = braintrust_group.engineers.id
  role_id     = braintrust_role.prod_reader.id
}

resource "braintrust_acl" "sre_prod" {
  object_type = "project"
  object_id   = braintrust_project.prod.id
  group_id    = braintrust_group.sre.id
  role_id     = braintrust_role.prod_operator.id
}

# --- Leads get full access everywhere ---
resource "braintrust_acl" "leads_dev" {
  object_type = "project"
  object_id   = braintrust_project.dev.id
  group_id    = braintrust_group.leads.id
  role_id     = braintrust_role.dev_engineer.id
}

resource "braintrust_acl" "leads_staging" {
  object_type = "project"
  object_id   = braintrust_project.staging.id
  group_id    = braintrust_group.leads.id
  role_id     = braintrust_role.dev_engineer.id
}

resource "braintrust_acl" "leads_prod" {
  object_type = "project"
  object_id   = braintrust_project.prod.id
  group_id    = braintrust_group.leads.id
  role_id     = braintrust_role.prod_operator.id
}

# =============================================================================
# 6. Service tokens per environment.
# =============================================================================

resource "braintrust_service_account" "ci_dev" {
  name = "ci-dev"
}

resource "braintrust_service_account" "ci_staging" {
  name = "ci-staging"
}

resource "braintrust_service_account" "ci_prod" {
  name = "ci-prod"
}

resource "braintrust_service_token" "ci_dev" {
  name               = "ci-dev"
  service_account_id = braintrust_service_account.ci_dev.id
}

resource "braintrust_service_token" "ci_staging" {
  name               = "ci-staging"
  service_account_id = braintrust_service_account.ci_staging.id
}

resource "braintrust_service_token" "ci_prod" {
  name               = "ci-prod"
  service_account_id = braintrust_service_account.ci_prod.id
}

# =============================================================================
# 7. Alerting — stricter thresholds in prod.
# =============================================================================

resource "braintrust_project_automation" "dev_alert" {
  project_id = braintrust_project.dev.id
  name       = "Dev Score Alert"

  logs_config = {
    btql_filter      = "scores.accuracy < 0.5"
    interval_seconds = 3600

    webhook_action = {
      url = var.slack_webhook_url
    }
  }
}

resource "braintrust_project_automation" "staging_alert" {
  project_id = braintrust_project.staging.id
  name       = "Staging Score Alert"

  logs_config = {
    btql_filter      = "scores.accuracy < 0.7"
    interval_seconds = 600

    webhook_action = {
      url = var.slack_webhook_url
    }
  }
}

resource "braintrust_project_automation" "prod_alert" {
  project_id  = braintrust_project.prod.id
  name        = "Prod Score Regression"
  description = "URGENT: production accuracy below threshold"

  logs_config = {
    btql_filter      = "scores.accuracy < 0.85"
    interval_seconds = 60

    webhook_action = {
      url = var.slack_webhook_url
    }
  }
}

# =============================================================================
# 8. Retention — longer in prod, shorter in dev.
# =============================================================================

resource "braintrust_project_automation" "dev_retention" {
  project_id = braintrust_project.dev.id
  name       = "Dev Log Retention 14d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 14
  }
}

resource "braintrust_project_automation" "staging_retention" {
  project_id = braintrust_project.staging.id
  name       = "Staging Log Retention 30d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 30
  }
}

resource "braintrust_project_automation" "prod_retention" {
  project_id = braintrust_project.prod.id
  name       = "Prod Log Retention 180d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 180
  }
}

# =============================================================================
# 9. AI credentials.
# =============================================================================

resource "braintrust_ai_secret" "openai" {
  name   = "OpenAI"
  type   = "openai"
  secret = var.openai_api_key
}

# =============================================================================
# Outputs
# =============================================================================

output "environment_project_ids" {
  value = {
    dev     = braintrust_project.dev.id
    staging = braintrust_project.staging.id
    prod    = braintrust_project.prod.id
  }
}

output "ci_tokens" {
  description = "Service tokens per environment — store as CI secrets"
  sensitive   = true
  value = {
    dev     = braintrust_service_token.ci_dev.key
    staging = braintrust_service_token.ci_staging.key
    prod    = braintrust_service_token.ci_prod.key
  }
}
