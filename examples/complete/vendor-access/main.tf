# =============================================================================
# External Vendor / Contractor Access
# =============================================================================
#
# Grant limited, scoped access to external parties:
#
#   - Labeling contractors: read datasets, update scores. NO access to logs
#     (which may contain user PII) or prompts (IP).
#   - Consulting partner: read-only on specific projects for advisory work.
#   - Vendor integration: service token scoped to push experiment results.
#
# Key principles:
#   - External parties get the minimum access needed.
#   - PII-containing resources (logs) are never exposed to contractors.
#   - Prompts (intellectual property) are never exposed to external parties.
#   - Service tokens for machine-to-machine access have narrow scope.
#   - All contractor access is grouped for easy revocation.
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

variable "internal_lead_emails" {
  description = "Internal team leads who manage the project"
  type        = list(string)
  default     = ["lead@company.com"]
}

variable "labeler_emails" {
  description = "External labeling contractors"
  type        = list(string)
  default     = ["labeler1@contractor.com", "labeler2@contractor.com"]
}

variable "consultant_emails" {
  description = "External AI consultants (read-only advisory)"
  type        = list(string)
  default     = ["advisor@consulting.com"]
}

variable "openai_api_key" {
  type      = string
  sensitive = true
}

# =============================================================================
# 1. Users
# =============================================================================

data "braintrust_user" "leads" {
  for_each = toset(var.internal_lead_emails)
  email    = each.value
}

data "braintrust_user" "labelers" {
  for_each = toset(var.labeler_emails)
  email    = each.value
}

data "braintrust_user" "consultants" {
  for_each = toset(var.consultant_emails)
  email    = each.value
}

# =============================================================================
# 2. Projects
# =============================================================================

resource "braintrust_project" "main" {
  name        = "Customer Chatbot"
  description = "Customer-facing chatbot — contains user interaction logs (PII)"
}

# =============================================================================
# 3. Custom roles — carefully scoped for external access.
# =============================================================================

# Labeler: read datasets + experiments, update experiment scores for labeling.
# Explicitly restricted to datasets and experiments only —
# NO access to project_log (PII) or prompt (IP).
resource "braintrust_role" "labeler" {
  name        = "External Labeler"
  description = "Read datasets and experiments, update scores for labeling. No access to logs or prompts."

  member_permissions = [
    { permission = "read", restrict_object_type = "dataset" },
    { permission = "read", restrict_object_type = "experiment" },
    { permission = "update", restrict_object_type = "experiment" },
  ]
}

# Consultant: read-only on experiments and datasets.
# No logs (PII), no prompts (IP), no write access.
resource "braintrust_role" "consultant" {
  name        = "External Consultant"
  description = "Read-only access to experiments and datasets for advisory. No access to logs or prompts."

  member_permissions = [
    { permission = "read", restrict_object_type = "experiment" },
    { permission = "read", restrict_object_type = "dataset" },
  ]
}

# Internal lead: full access + ACL management.
resource "braintrust_role" "project_owner" {
  name        = "Project Owner"
  description = "Full access including permission management."

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
# 4. Groups — internal vs external, for easy bulk revocation.
# =============================================================================

resource "braintrust_group" "internal_leads" {
  name         = "Internal Leads"
  description  = "Internal team leads."
  member_users = [for u in data.braintrust_user.leads : u.id]
}

# External groups are separate so you can revoke all contractor access
# by deleting the ACL or the group — no need to touch individual users.
resource "braintrust_group" "labelers" {
  name         = "External Labelers"
  description  = "Labeling contractor team. Remove this group to revoke all labeler access."
  member_users = [for u in data.braintrust_user.labelers : u.id]
}

resource "braintrust_group" "consultants" {
  name         = "External Consultants"
  description  = "External advisory consultants. Remove this group to revoke access."
  member_users = [for u in data.braintrust_user.consultants : u.id]
}

# =============================================================================
# 5. ACLs
# =============================================================================

resource "braintrust_acl" "leads" {
  object_type = "project"
  object_id   = braintrust_project.main.id
  group_id    = braintrust_group.internal_leads.id
  role_id     = braintrust_role.project_owner.id
}

resource "braintrust_acl" "labelers" {
  object_type = "project"
  object_id   = braintrust_project.main.id
  group_id    = braintrust_group.labelers.id
  role_id     = braintrust_role.labeler.id
}

resource "braintrust_acl" "consultants" {
  object_type = "project"
  object_id   = braintrust_project.main.id
  group_id    = braintrust_group.consultants.id
  role_id     = braintrust_role.consultant.id
}

# =============================================================================
# 6. Vendor integration — service token for machine-to-machine access.
# =============================================================================

# Create a service account for vendor integration automation.
resource "braintrust_service_account" "vendor_integration" {
  name = "vendor-integration"
}

# This token is shared with a vendor's system to push eval results.
# The token inherits the permissions of the service account, which
# we'd scope via ACLs in a production setup.
resource "braintrust_service_token" "vendor_integration" {
  name               = "vendor-eval-pusher"
  service_account_id = braintrust_service_account.vendor_integration.id
}

# =============================================================================
# 7. Project scaffolding — datasets and scores for labeling workflow.
# =============================================================================

resource "braintrust_dataset" "labeling_queue" {
  project_id  = braintrust_project.main.id
  name        = "Labeling Queue"
  description = "Samples queued for human labeling by contractors"
}

resource "braintrust_dataset" "golden_set" {
  project_id  = braintrust_project.main.id
  name        = "Golden Test Set"
  description = "Verified high-quality test cases"
}

resource "braintrust_project_score" "helpfulness" {
  project_id  = braintrust_project.main.id
  name        = "Helpfulness"
  score_type  = "slider"
  description = "How helpful was the chatbot response?"
}

resource "braintrust_project_score" "safety" {
  project_id  = braintrust_project.main.id
  name        = "Safety"
  score_type  = "categorical"
  description = "Content safety rating"
}

resource "braintrust_project_score" "accuracy" {
  project_id  = braintrust_project.main.id
  name        = "Accuracy"
  score_type  = "slider"
  description = "Factual accuracy of the response"
}

# =============================================================================
# 8. AI credentials + retention.
# =============================================================================

resource "braintrust_ai_secret" "openai" {
  name   = "OpenAI"
  type   = "openai"
  secret = var.openai_api_key
}

resource "braintrust_project_automation" "retention" {
  project_id = braintrust_project.main.id
  name       = "Log Retention 90d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 90
  }
}
