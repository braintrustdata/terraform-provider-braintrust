# =============================================================================
# Startup Quick Start
# =============================================================================
#
# The simplest useful setup for a small team (5–15 people) with one or two
# AI products. Gets you from zero to a working Braintrust org in minutes.
#
# Assumptions:
#   - One small team, no cross-team isolation needed yet.
#   - A founder/CTO who needs full control.
#   - A handful of engineers who build and iterate.
#   - A product manager who reviews results but shouldn't delete anything.
#   - One or two AI products (projects).
#
# This uses built-in roles where possible and only creates ONE custom role
# (for the PM). Copy this, fill in emails, and `terraform apply`.
# =============================================================================

terraform {
  required_providers {
    braintrust = {
      source = "braintrustdata/braintrust"
    }
  }
}

provider "braintrust" {}

# =============================================================================
# Variables — fill in your team's emails.
# =============================================================================

variable "admin_emails" {
  description = "Founder / CTO / team leads with full org access"
  type        = list(string)
  default     = ["cto@startup.com"]
}

variable "engineer_emails" {
  description = "Engineers who build and iterate on AI features"
  type        = list(string)
  default     = ["eng1@startup.com", "eng2@startup.com", "eng3@startup.com"]
}

variable "pm_emails" {
  description = "Product managers who review results (read + create experiments)"
  type        = list(string)
  default     = ["pm@startup.com"]
}

variable "openai_api_key" {
  type      = string
  sensitive = true
}

variable "anthropic_api_key" {
  type      = string
  sensitive = true
  default   = ""
}

# =============================================================================
# 1. Look up users by email.
# =============================================================================

data "braintrust_user" "admins" {
  for_each = toset(var.admin_emails)
  email    = each.value
}

data "braintrust_user" "engineers" {
  for_each = toset(var.engineer_emails)
  email    = each.value
}

data "braintrust_user" "pms" {
  for_each = toset(var.pm_emails)
  email    = each.value
}

# =============================================================================
# 2. Projects — one per product / use case.
# =============================================================================

resource "braintrust_project" "main_product" {
  name        = "AI Assistant"
  description = "Our core AI assistant product"
}

resource "braintrust_project" "internal_tools" {
  name        = "Internal Tools"
  description = "Internal AI-powered tools and automations"
}

# =============================================================================
# 3. One custom role: Product Manager.
#
#    Built-in roles cover admins (Owner) and engineers (Engineer), but PMs
#    need something in between: read everything, create experiments to run
#    evals, but no delete and no infrastructure changes.
# =============================================================================

resource "braintrust_role" "product_manager" {
  name        = "Product Manager"
  description = "Read everything, create/update experiments. Cannot delete or manage access."

  member_permissions = [
    { permission = "read" },
    { permission = "create", restrict_object_type = "experiment" },
    { permission = "update", restrict_object_type = "experiment" },
  ]
}

# =============================================================================
# 4. Groups — keep it simple: admins, engineers, PMs.
# =============================================================================

resource "braintrust_group" "admins" {
  name         = "Admins"
  description  = "Founders and team leads with full access."
  member_users = [for u in data.braintrust_user.admins : u.id]
}

resource "braintrust_group" "engineers" {
  name         = "Engineers"
  description  = "Engineers who build AI features."
  member_users = [for u in data.braintrust_user.engineers : u.id]
}

resource "braintrust_group" "pms" {
  name         = "Product Managers"
  description  = "PMs who review eval results."
  member_users = [for u in data.braintrust_user.pms : u.id]
}

# =============================================================================
# 5. ACLs — use built-in roles for admins and engineers.
#
#    We grant at org_project level so the permissions apply to ALL projects.
#    For a small team this is fine — you don't need per-project isolation yet.
# =============================================================================

# Look up the built-in roles.
data "braintrust_role" "owner" {
  name = "Owner"
}

data "braintrust_role" "engineer" {
  name = "Engineer"
}

# Admins get the Owner role on the whole organization.
resource "braintrust_acl" "admins_org" {
  object_type = "organization"
  object_id   = braintrust_project.main_product.org_id
  group_id    = braintrust_group.admins.id
  role_id     = data.braintrust_role.owner.id
}

# Engineers get the built-in Engineer role on all projects.
resource "braintrust_acl" "engineers_projects" {
  object_type = "org_project"
  object_id   = braintrust_project.main_product.org_id
  group_id    = braintrust_group.engineers.id
  role_id     = data.braintrust_role.engineer.id
}

# PMs get our custom Product Manager role on all projects.
resource "braintrust_acl" "pms_projects" {
  object_type = "org_project"
  object_id   = braintrust_project.main_product.org_id
  group_id    = braintrust_group.pms.id
  role_id     = braintrust_role.product_manager.id
}

# =============================================================================
# 6. AI provider credentials.
# =============================================================================

resource "braintrust_ai_secret" "openai" {
  name   = "OpenAI"
  type   = "openai"
  secret = var.openai_api_key
}

resource "braintrust_ai_secret" "anthropic" {
  count  = var.anthropic_api_key != "" ? 1 : 0
  name   = "Anthropic"
  type   = "anthropic"
  secret = var.anthropic_api_key
}

# =============================================================================
# 7. Basic project scaffolding.
# =============================================================================

resource "braintrust_project_score" "quality" {
  project_id  = braintrust_project.main_product.id
  name        = "Quality"
  score_type  = "slider"
  description = "Overall response quality"
}

resource "braintrust_project_score" "accuracy" {
  project_id  = braintrust_project.main_product.id
  name        = "Accuracy"
  score_type  = "slider"
  description = "Factual accuracy of the response"
}

resource "braintrust_project_tag" "baseline" {
  project_id  = braintrust_project.main_product.id
  name        = "baseline"
  description = "Baseline evaluation runs"
}

resource "braintrust_dataset" "eval_set" {
  project_id  = braintrust_project.main_product.id
  name        = "Evaluation Dataset"
  description = "Core evaluation dataset for regression testing"
}

# =============================================================================
# Outputs
# =============================================================================

output "project_ids" {
  value = {
    main_product   = braintrust_project.main_product.id
    internal_tools = braintrust_project.internal_tools.id
  }
}
