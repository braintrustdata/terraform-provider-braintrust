# =============================================================================
# Compliance & Audit — Separation of Duties
# =============================================================================
#
# For regulated industries (healthcare, finance, government) where:
#
#   - Developers cannot approve their own evaluations.
#   - A compliance team has read + ACL visibility across everything.
#   - A red team can run adversarial evals but cannot modify prompts.
#   - Reviewers can label/score but cannot modify experiments or data.
#   - Data retention is enforced on every project.
#   - All access is auditable via ACL read permissions.
#
# This implements the "segregation of duties" principle recommended by
# NIST AI RMF and ISO/IEC 42001 governance frameworks.
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
# Variables
# =============================================================================

variable "compliance_emails" {
  description = "Compliance officers — read + audit access to everything"
  type        = list(string)
  default     = ["compliance@corp.com"]
}

variable "developer_emails" {
  description = "AI developers — build experiments, cannot delete or approve"
  type        = list(string)
  default     = ["dev1@corp.com", "dev2@corp.com"]
}

variable "reviewer_emails" {
  description = "Human reviewers — label and score outputs, cannot modify experiments"
  type        = list(string)
  default     = ["reviewer1@corp.com", "reviewer2@corp.com"]
}

variable "red_team_emails" {
  description = "Red team — run adversarial evals, no prompt access"
  type        = list(string)
  default     = ["redteam@corp.com"]
}

variable "admin_emails" {
  description = "Platform admins — full access"
  type        = list(string)
  default     = ["admin@corp.com"]
}

variable "retention_days" {
  description = "Mandatory data retention period (days) for all projects"
  type        = number
  default     = 90
}

# =============================================================================
# 1. Users
# =============================================================================

data "braintrust_user" "compliance" {
  for_each = toset(var.compliance_emails)
  email    = each.value
}

data "braintrust_user" "developers" {
  for_each = toset(var.developer_emails)
  email    = each.value
}

data "braintrust_user" "reviewers" {
  for_each = toset(var.reviewer_emails)
  email    = each.value
}

data "braintrust_user" "red_team" {
  for_each = toset(var.red_team_emails)
  email    = each.value
}

data "braintrust_user" "admins" {
  for_each = toset(var.admin_emails)
  email    = each.value
}

# =============================================================================
# 2. Projects
# =============================================================================

resource "braintrust_project" "patient_triage" {
  name        = "Patient Triage AI"
  description = "Clinical decision support — regulated, requires audit trail"
}

resource "braintrust_project" "claims_processing" {
  name        = "Claims Processing"
  description = "Insurance claims automation — PII handling required"
}

locals {
  regulated_projects = {
    patient_triage    = braintrust_project.patient_triage
    claims_processing = braintrust_project.claims_processing
  }
}

# =============================================================================
# 3. Custom roles — designed around separation of duties.
# =============================================================================

# Compliance Officer: read everything + see who has access to what.
# Cannot modify anything — pure audit/oversight role.
resource "braintrust_role" "compliance_officer" {
  name        = "Compliance Officer"
  description = "Read-only access to all resources plus visibility into access control. Cannot modify anything."

  member_permissions = [
    { permission = "read" },
    { permission = "read_acls" },
  ]
}

# Developer: create and update experiments and datasets.
# CANNOT delete (prevents data destruction).
# CANNOT modify prompts (prompt changes require lead approval).
# CANNOT manage access.
resource "braintrust_role" "developer" {
  name        = "Developer"
  description = "Create/update experiments and datasets. Cannot delete, modify prompts, or manage access."

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

# Reviewer: read experiments + update scores/logs (labeling).
# Cannot create experiments, modify datasets, or touch prompts.
resource "braintrust_role" "reviewer" {
  name        = "Reviewer"
  description = "Read all resources, update experiment scores and logs for labeling. Cannot create or delete."

  member_permissions = [
    { permission = "read" },
    { permission = "update", restrict_object_type = "experiment" },
    { permission = "update", restrict_object_type = "project_log" },
  ]
}

# Red Team: create adversarial experiments, read results.
# Explicitly NO access to prompts (IP protection).
# Cannot modify existing experiments (tamper prevention).
resource "braintrust_role" "red_team" {
  name        = "Red Team"
  description = "Run adversarial evaluations. Can create experiments but cannot access prompts or modify existing data."

  member_permissions = [
    { permission = "read", restrict_object_type = "experiment" },
    { permission = "read", restrict_object_type = "dataset" },
    { permission = "read", restrict_object_type = "project_log" },
    { permission = "create", restrict_object_type = "experiment" },
    { permission = "create", restrict_object_type = "project_log" },
  ]
}

# Platform Admin: full access including ACL management.
resource "braintrust_role" "platform_admin" {
  name        = "Platform Admin"
  description = "Full access to all resources and permission management."

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

resource "braintrust_group" "compliance" {
  name         = "Compliance"
  description  = "Compliance officers and auditors."
  member_users = [for u in data.braintrust_user.compliance : u.id]
}

resource "braintrust_group" "developers" {
  name         = "Developers"
  description  = "AI/ML developers."
  member_users = [for u in data.braintrust_user.developers : u.id]
}

resource "braintrust_group" "reviewers" {
  name         = "Human Reviewers"
  description  = "Human reviewers for labeling and scoring."
  member_users = [for u in data.braintrust_user.reviewers : u.id]
}

resource "braintrust_group" "red_team" {
  name         = "Red Team"
  description  = "Adversarial evaluation specialists."
  member_users = [for u in data.braintrust_user.red_team : u.id]
}

resource "braintrust_group" "admins" {
  name         = "Platform Admins"
  description  = "Platform administrators."
  member_users = [for u in data.braintrust_user.admins : u.id]
}

# =============================================================================
# 5. ACLs — bind groups to roles on each regulated project.
# =============================================================================

# Compliance gets read + audit on ALL projects (org_project level).
resource "braintrust_acl" "compliance_all" {
  object_type = "org_project"
  object_id   = braintrust_project.patient_triage.org_id
  group_id    = braintrust_group.compliance.id
  role_id     = braintrust_role.compliance_officer.id
}

# Admins get full access on ALL projects.
resource "braintrust_acl" "admins_all" {
  object_type = "org_project"
  object_id   = braintrust_project.patient_triage.org_id
  group_id    = braintrust_group.admins.id
  role_id     = braintrust_role.platform_admin.id
}

# Developers, reviewers, and red team get per-project access.
resource "braintrust_acl" "developers" {
  for_each    = local.regulated_projects
  object_type = "project"
  object_id   = each.value.id
  group_id    = braintrust_group.developers.id
  role_id     = braintrust_role.developer.id
}

resource "braintrust_acl" "reviewers" {
  for_each    = local.regulated_projects
  object_type = "project"
  object_id   = each.value.id
  group_id    = braintrust_group.reviewers.id
  role_id     = braintrust_role.reviewer.id
}

resource "braintrust_acl" "red_team" {
  for_each    = local.regulated_projects
  object_type = "project"
  object_id   = each.value.id
  group_id    = braintrust_group.red_team.id
  role_id     = braintrust_role.red_team.id
}

# =============================================================================
# 6. Mandatory data retention on every project.
# =============================================================================

resource "braintrust_project_automation" "log_retention" {
  for_each   = local.regulated_projects
  project_id = each.value.id
  name       = "Mandatory Log Retention ${var.retention_days}d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = var.retention_days
  }
}

resource "braintrust_project_automation" "experiment_retention" {
  for_each   = local.regulated_projects
  project_id = each.value.id
  name       = "Experiment Retention ${var.retention_days}d"

  retention_config = {
    object_type    = "experiment"
    retention_days = var.retention_days
  }
}
