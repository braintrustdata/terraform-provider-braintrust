# =============================================================================
# Team-Based Access Control for Braintrust
# =============================================================================
#
# This example sets up a realistic multi-team permission model where:
#
#   - Projects map to use cases (e.g. "RAG Pipeline", "Customer Support Bot"),
#     NOT to teams.
#   - Each team "owns" one or more projects.
#   - Teams CANNOT see other teams' projects.
#   - Within each team, there are three tiers of access:
#       1. Viewers   — read-only across the project
#       2. Engineers  — create/read/update experiments, datasets, prompts, logs
#       3. Leads     — full CRUD + manage access (ACLs)
#
# Hierarchy reminder (permissions cascade downward):
#
#   organization
#     └── org_project  (virtual — "all projects")
#           └── project
#                 ├── experiment
#                 ├── dataset
#                 ├── prompt
#                 ├── prompt_session
#                 └── project_log
#
# We deliberately avoid granting anything at `org_project` so that teams
# are isolated by default. Each team's groups get ACLs scoped to their
# specific project(s).
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

# =============================================================================
# Variables
# =============================================================================

variable "ml_team_members" {
  description = "ML Platform team members (email addresses)"
  type = object({
    leads     = list(string)
    engineers = list(string)
    viewers   = list(string)
  })
  default = {
    leads     = ["alice@company.com"]
    engineers = ["bob@company.com", "carol@company.com"]
    viewers   = ["dave@company.com"]
  }
}

variable "product_team_members" {
  description = "Product AI team members (email addresses)"
  type = object({
    leads     = list(string)
    engineers = list(string)
    viewers   = list(string)
  })
  default = {
    leads     = ["eve@company.com"]
    engineers = ["frank@company.com"]
    viewers   = ["grace@company.com", "heidi@company.com"]
  }
}

# =============================================================================
# 1. Look up users by email so we never hardcode UUIDs.
# =============================================================================

# --- ML Platform team ---

data "braintrust_user" "ml_leads" {
  for_each = toset(var.ml_team_members.leads)
  email    = each.value
}

data "braintrust_user" "ml_engineers" {
  for_each = toset(var.ml_team_members.engineers)
  email    = each.value
}

data "braintrust_user" "ml_viewers" {
  for_each = toset(var.ml_team_members.viewers)
  email    = each.value
}

# --- Product AI team ---

data "braintrust_user" "product_leads" {
  for_each = toset(var.product_team_members.leads)
  email    = each.value
}

data "braintrust_user" "product_engineers" {
  for_each = toset(var.product_team_members.engineers)
  email    = each.value
}

data "braintrust_user" "product_viewers" {
  for_each = toset(var.product_team_members.viewers)
  email    = each.value
}

# =============================================================================
# 2. Projects — one per use case, NOT per team.
# =============================================================================

resource "braintrust_project" "rag_pipeline" {
  name        = "RAG Pipeline"
  description = "Retrieval-augmented generation evaluation and monitoring"
}

resource "braintrust_project" "support_bot" {
  name        = "Customer Support Bot"
  description = "Customer-facing support agent quality tracking"
}

resource "braintrust_project" "content_moderation" {
  name        = "Content Moderation"
  description = "Automated content moderation model evaluation"
}

# =============================================================================
# 3. Custom roles — fine-grained access tiers.
# =============================================================================
#
# We create custom roles instead of using the built-in ones because:
#   - Built-in "Engineer" grants CRUD on everything — we want more nuance.
#   - Built-in roles are applied at org_project level — we need project-scoped.
#
# Role: "Project Viewer"
#   - Read everything in a project (experiments, datasets, prompts, logs).
#   - Cannot create, update, or delete anything.
#
# Role: "Project Engineer"
#   - Read everything.
#   - Create/update experiments, datasets, prompts, logs.
#   - Cannot delete (prevents accidental data loss).
#   - Cannot manage access (ACLs).
#
# Role: "Project Lead"
#   - Full CRUD on everything in the project.
#   - Can manage access (create/read/update/delete ACLs).
#   - This is the "owner" tier for a team's project.

resource "braintrust_role" "project_viewer" {
  name        = "Project Viewer"
  description = "Read-only access to all resources within a project."

  member_permissions = [
    { permission = "read" },
  ]
}

resource "braintrust_role" "project_engineer" {
  name        = "Project Engineer"
  description = "Create and update experiments, datasets, prompts, and logs. Cannot delete or manage access."

  member_permissions = [
    # Read everything.
    { permission = "read" },

    # Create + update, scoped to each resource type.
    # We scope these individually so engineers can't create/update the project
    # itself (e.g. rename it or change settings), only work within it.
    { permission = "create", restrict_object_type = "experiment" },
    { permission = "update", restrict_object_type = "experiment" },
    { permission = "create", restrict_object_type = "dataset" },
    { permission = "update", restrict_object_type = "dataset" },
    { permission = "create", restrict_object_type = "prompt" },
    { permission = "update", restrict_object_type = "prompt" },
    { permission = "create", restrict_object_type = "project_log" },
    { permission = "update", restrict_object_type = "project_log" },
  ]
}

resource "braintrust_role" "project_lead" {
  name        = "Project Lead"
  description = "Full access to all resources within a project, including managing permissions."

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
# 4. Groups — one per (team × tier).
# =============================================================================
#
# Each team gets three groups. Users belong to exactly one group per team.
# Groups are the targets of ACL grants — we never grant to individual users.

# --- ML Platform team ---

resource "braintrust_group" "ml_leads" {
  name         = "ML Platform — Leads"
  description  = "ML Platform team leads with full project access."
  member_users = [for u in data.braintrust_user.ml_leads : u.id]
}

resource "braintrust_group" "ml_engineers" {
  name         = "ML Platform — Engineers"
  description  = "ML Platform engineers who build and iterate on experiments."
  member_users = [for u in data.braintrust_user.ml_engineers : u.id]
}

resource "braintrust_group" "ml_viewers" {
  name         = "ML Platform — Viewers"
  description  = "ML Platform stakeholders with read-only access."
  member_users = [for u in data.braintrust_user.ml_viewers : u.id]
}

# --- Product AI team ---

resource "braintrust_group" "product_leads" {
  name         = "Product AI — Leads"
  description  = "Product AI team leads with full project access."
  member_users = [for u in data.braintrust_user.product_leads : u.id]
}

resource "braintrust_group" "product_engineers" {
  name         = "Product AI — Engineers"
  description  = "Product AI engineers who build and iterate on experiments."
  member_users = [for u in data.braintrust_user.product_engineers : u.id]
}

resource "braintrust_group" "product_viewers" {
  name         = "Product AI — Viewers"
  description  = "Product AI stakeholders with read-only access."
  member_users = [for u in data.braintrust_user.product_viewers : u.id]
}

# =============================================================================
# 5. ACLs — bind groups to roles on specific projects.
# =============================================================================
#
# This is where isolation happens. Each ACL is scoped to:
#   object_type = "project"
#   object_id   = <specific project ID>
#
# Because we grant at the project level (not org_project), permissions
# cascade to experiments, datasets, prompts, and logs within that project —
# but NOT to other projects.
#
# A team that is NOT granted any ACL on a project simply cannot see it.

# ---------------------------------------------------------------------------
# ML Platform team → RAG Pipeline project
# ---------------------------------------------------------------------------

resource "braintrust_acl" "ml_leads_rag" {
  object_type = "project"
  object_id   = braintrust_project.rag_pipeline.id
  group_id    = braintrust_group.ml_leads.id
  role_id     = braintrust_role.project_lead.id
}

resource "braintrust_acl" "ml_engineers_rag" {
  object_type = "project"
  object_id   = braintrust_project.rag_pipeline.id
  group_id    = braintrust_group.ml_engineers.id
  role_id     = braintrust_role.project_lead.id
}

resource "braintrust_acl" "ml_viewers_rag" {
  object_type = "project"
  object_id   = braintrust_project.rag_pipeline.id
  group_id    = braintrust_group.ml_viewers.id
  role_id     = braintrust_role.project_viewer.id
}

# ---------------------------------------------------------------------------
# ML Platform team → Content Moderation project
# ---------------------------------------------------------------------------

resource "braintrust_acl" "ml_leads_moderation" {
  object_type = "project"
  object_id   = braintrust_project.content_moderation.id
  group_id    = braintrust_group.ml_leads.id
  role_id     = braintrust_role.project_lead.id
}

resource "braintrust_acl" "ml_engineers_moderation" {
  object_type = "project"
  object_id   = braintrust_project.content_moderation.id
  group_id    = braintrust_group.ml_engineers.id
  role_id     = braintrust_role.project_engineer.id
}

resource "braintrust_acl" "ml_viewers_moderation" {
  object_type = "project"
  object_id   = braintrust_project.content_moderation.id
  group_id    = braintrust_group.ml_viewers.id
  role_id     = braintrust_role.project_viewer.id
}

# ---------------------------------------------------------------------------
# Product AI team → Customer Support Bot project
# ---------------------------------------------------------------------------

resource "braintrust_acl" "product_leads_support" {
  object_type = "project"
  object_id   = braintrust_project.support_bot.id
  group_id    = braintrust_group.product_leads.id
  role_id     = braintrust_role.project_lead.id
}

resource "braintrust_acl" "product_engineers_support" {
  object_type = "project"
  object_id   = braintrust_project.support_bot.id
  group_id    = braintrust_group.product_engineers.id
  role_id     = braintrust_role.project_engineer.id
}

resource "braintrust_acl" "product_viewers_support" {
  object_type = "project"
  object_id   = braintrust_project.support_bot.id
  group_id    = braintrust_group.product_viewers.id
  role_id     = braintrust_role.project_viewer.id
}

# ---------------------------------------------------------------------------
# Cross-team access: Product AI viewers get read-only on RAG Pipeline.
# This shows how to grant limited cross-team visibility without
# opening up everything.
# ---------------------------------------------------------------------------

resource "braintrust_acl" "product_viewers_rag_readonly" {
  object_type = "project"
  object_id   = braintrust_project.rag_pipeline.id
  group_id    = braintrust_group.product_viewers.id
  role_id     = braintrust_role.project_viewer.id
}

# =============================================================================
# 6. Project infrastructure — scores, tags, datasets, retention.
# =============================================================================
#
# These are set up by the team leads as part of project scaffolding.
# Because the ACLs above grant per-project access, only the appropriate
# team members can actually use these resources.

# --- RAG Pipeline project scaffolding ---

resource "braintrust_project_score" "rag_relevance" {
  project_id  = braintrust_project.rag_pipeline.id
  name        = "Relevance"
  score_type  = "slider"
  description = "How relevant is the retrieved context to the question?"
}

resource "braintrust_project_score" "rag_faithfulness" {
  project_id  = braintrust_project.rag_pipeline.id
  name        = "Faithfulness"
  score_type  = "slider"
  description = "Does the answer stay faithful to the retrieved context?"
}

resource "braintrust_project_tag" "rag_production" {
  project_id  = braintrust_project.rag_pipeline.id
  name        = "production"
  description = "Production traffic samples"
}

resource "braintrust_project_tag" "rag_regression" {
  project_id  = braintrust_project.rag_pipeline.id
  name        = "regression"
  description = "Regression test runs"
}

resource "braintrust_dataset" "rag_golden" {
  project_id  = braintrust_project.rag_pipeline.id
  name        = "Golden QA Pairs"
  description = "Curated question/answer pairs for regression testing"
}

# --- Customer Support Bot project scaffolding ---

resource "braintrust_project_score" "support_quality" {
  project_id  = braintrust_project.support_bot.id
  name        = "Response Quality"
  score_type  = "slider"
  description = "Overall quality of the bot response"
}

resource "braintrust_project_score" "support_resolution" {
  project_id  = braintrust_project.support_bot.id
  name        = "Resolution"
  score_type  = "categorical"
  description = "Did the bot resolve the customer issue?"
}

resource "braintrust_dataset" "support_test_cases" {
  project_id  = braintrust_project.support_bot.id
  name        = "Support Test Cases"
  description = "Real customer interactions for evaluation"
}

# --- Log retention for all projects ---

resource "braintrust_project_automation" "rag_retention" {
  project_id = braintrust_project.rag_pipeline.id
  name       = "Log Retention 90d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 90
  }
}

resource "braintrust_project_automation" "support_retention" {
  project_id = braintrust_project.support_bot.id
  name       = "Log Retention 60d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 60
  }
}

# =============================================================================
# 7. AI provider credentials — org-level, shared across all projects.
# =============================================================================

variable "openai_api_key" {
  type      = string
  sensitive = true
}

resource "braintrust_ai_secret" "openai" {
  name   = "OpenAI"
  type   = "openai"
  secret = var.openai_api_key
}

# =============================================================================
# Outputs — useful for downstream automation.
# =============================================================================

output "project_ids" {
  description = "Map of project name → ID"
  value = {
    rag_pipeline       = braintrust_project.rag_pipeline.id
    support_bot        = braintrust_project.support_bot.id
    content_moderation = braintrust_project.content_moderation.id
  }
}

output "group_ids" {
  description = "Map of group name → ID"
  value = {
    ml_leads          = braintrust_group.ml_leads.id
    ml_engineers      = braintrust_group.ml_engineers.id
    ml_viewers        = braintrust_group.ml_viewers.id
    product_leads     = braintrust_group.product_leads.id
    product_engineers = braintrust_group.product_engineers.id
    product_viewers   = braintrust_group.product_viewers.id
  }
}

output "role_ids" {
  description = "Map of custom role name → ID"
  value = {
    project_viewer   = braintrust_role.project_viewer.id
    project_engineer = braintrust_role.project_engineer.id
    project_lead     = braintrust_role.project_lead.id
  }
}
