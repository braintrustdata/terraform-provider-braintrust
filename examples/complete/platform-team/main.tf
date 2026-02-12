# =============================================================================
# Hub-and-Spoke: Central Platform Team + Product Teams
# =============================================================================
#
# A central ML Platform / MLOps team provides shared infrastructure while
# product teams independently own their evaluation projects:
#
#   Platform team:
#     - Admin access to ALL projects (via org_project).
#     - Manages AI credentials, standard scores, retention policies.
#     - Sets org-wide guardrails.
#
#   Product teams (isolated from each other):
#     - Own their projects with full CRUD.
#     - Cannot see other product teams' projects.
#     - Inherit standardized scores and tags from the platform team.
#
# This is the "scaling AI org" pattern: centralized governance with
# decentralized execution.
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

variable "platform_emails" {
  description = "ML Platform / MLOps team members"
  type        = list(string)
  default     = ["platform1@company.com", "platform2@company.com"]
}

variable "search_team_emails" {
  description = "Search product team"
  type        = list(string)
  default     = ["search1@company.com", "search2@company.com"]
}

variable "chat_team_emails" {
  description = "Chat product team"
  type        = list(string)
  default     = ["chat1@company.com", "chat2@company.com"]
}

variable "recs_team_emails" {
  description = "Recommendations product team"
  type        = list(string)
  default     = ["recs1@company.com"]
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
# 1. Users
# =============================================================================

data "braintrust_user" "platform" {
  for_each = toset(var.platform_emails)
  email    = each.value
}

data "braintrust_user" "search" {
  for_each = toset(var.search_team_emails)
  email    = each.value
}

data "braintrust_user" "chat" {
  for_each = toset(var.chat_team_emails)
  email    = each.value
}

data "braintrust_user" "recs" {
  for_each = toset(var.recs_team_emails)
  email    = each.value
}

# =============================================================================
# 2. Projects — one per product.
# =============================================================================

resource "braintrust_project" "search" {
  name        = "Search"
  description = "Search relevance and ranking evaluation"
}

resource "braintrust_project" "chat" {
  name        = "Chat"
  description = "Conversational AI quality evaluation"
}

resource "braintrust_project" "recs" {
  name        = "Recommendations"
  description = "Recommendation engine accuracy evaluation"
}

locals {
  all_projects = {
    search = braintrust_project.search
    chat   = braintrust_project.chat
    recs   = braintrust_project.recs
  }

  # Map product teams to their projects.
  team_project_map = {
    search = { project = braintrust_project.search, members = data.braintrust_user.search }
    chat   = { project = braintrust_project.chat, members = data.braintrust_user.chat }
    recs   = { project = braintrust_project.recs, members = data.braintrust_user.recs }
  }
}

# =============================================================================
# 3. Roles
# =============================================================================

# Platform admin: full access + ACL management across all projects.
resource "braintrust_role" "platform_admin" {
  name        = "Platform Admin"
  description = "Full access to all resources and permission management. For the central platform team."

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

# Product team member: full CRUD within their project, no ACL management.
resource "braintrust_role" "product_engineer" {
  name        = "Product Engineer"
  description = "Full CRUD within a project. Cannot manage permissions."

  member_permissions = [
    { permission = "create" },
    { permission = "read" },
    { permission = "update" },
    { permission = "delete" },
  ]
}

# =============================================================================
# 4. Groups
# =============================================================================

resource "braintrust_group" "platform" {
  name         = "ML Platform"
  description  = "Central ML Platform / MLOps team."
  member_users = [for u in data.braintrust_user.platform : u.id]
}

resource "braintrust_group" "search_team" {
  name         = "Search Team"
  description  = "Search product team."
  member_users = [for u in data.braintrust_user.search : u.id]
}

resource "braintrust_group" "chat_team" {
  name         = "Chat Team"
  description  = "Chat product team."
  member_users = [for u in data.braintrust_user.chat : u.id]
}

resource "braintrust_group" "recs_team" {
  name         = "Recs Team"
  description  = "Recommendations product team."
  member_users = [for u in data.braintrust_user.recs : u.id]
}

# =============================================================================
# 5. ACLs — hub-and-spoke pattern.
# =============================================================================

# Platform team: admin on ALL projects via org_project.
resource "braintrust_acl" "platform_all" {
  object_type = "org_project"
  object_id   = braintrust_project.search.org_id
  group_id    = braintrust_group.platform.id
  role_id     = braintrust_role.platform_admin.id
}

# Product teams: full CRUD on ONLY their own project.
resource "braintrust_acl" "search_team" {
  object_type = "project"
  object_id   = braintrust_project.search.id
  group_id    = braintrust_group.search_team.id
  role_id     = braintrust_role.product_engineer.id
}

resource "braintrust_acl" "chat_team" {
  object_type = "project"
  object_id   = braintrust_project.chat.id
  group_id    = braintrust_group.chat_team.id
  role_id     = braintrust_role.product_engineer.id
}

resource "braintrust_acl" "recs_team" {
  object_type = "project"
  object_id   = braintrust_project.recs.id
  group_id    = braintrust_group.recs_team.id
  role_id     = braintrust_role.product_engineer.id
}

# =============================================================================
# 6. Standardized scores — platform team enforces consistency.
#    Every project gets the same core metrics.
# =============================================================================

resource "braintrust_project_score" "accuracy" {
  for_each    = local.all_projects
  project_id  = each.value.id
  name        = "Accuracy"
  score_type  = "slider"
  description = "Factual correctness (0–1)"
}

resource "braintrust_project_score" "latency" {
  for_each    = local.all_projects
  project_id  = each.value.id
  name        = "Latency"
  score_type  = "slider"
  description = "Response latency normalized (0–1, lower is better)"
}

resource "braintrust_project_score" "safety" {
  for_each    = local.all_projects
  project_id  = each.value.id
  name        = "Safety"
  score_type  = "categorical"
  description = "Content safety classification"
}

resource "braintrust_project_score" "cost" {
  for_each    = local.all_projects
  project_id  = each.value.id
  name        = "Cost"
  score_type  = "slider"
  description = "Inference cost per request normalized (0–1, lower is better)"
}

# =============================================================================
# 7. Standardized tags.
# =============================================================================

resource "braintrust_project_tag" "ci" {
  for_each    = local.all_projects
  project_id  = each.value.id
  name        = "ci"
  description = "CI pipeline run"
}

resource "braintrust_project_tag" "production" {
  for_each    = local.all_projects
  project_id  = each.value.id
  name        = "production"
  description = "Production traffic sample"
}

resource "braintrust_project_tag" "baseline" {
  for_each    = local.all_projects
  project_id  = each.value.id
  name        = "baseline"
  description = "Baseline comparison"
}

# =============================================================================
# 8. Data retention — org-wide policy applied to every project.
# =============================================================================

resource "braintrust_project_automation" "retention" {
  for_each   = local.all_projects
  project_id = each.value.id
  name       = "Log Retention 90d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 90
  }
}

# =============================================================================
# 9. AI credentials — shared org-wide by the platform team.
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
# Outputs
# =============================================================================

output "project_ids" {
  value = { for k, v in local.all_projects : k => v.id }
}

output "group_ids" {
  value = {
    platform = braintrust_group.platform.id
    search   = braintrust_group.search_team.id
    chat     = braintrust_group.chat_team.id
    recs     = braintrust_group.recs_team.id
  }
}
