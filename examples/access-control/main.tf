# =============================================================================
# Access Control
# =============================================================================
#
# A permission model for an org with several product teams:
#
#   Organization
#   ├── Admins       → built-in Owner role on the organization
#   ├── Auditors     → read data and ACLs on every team project (via a project group)
#   ├── Each team (one project per team, isolated from other teams)
#   │   ├── Leads      → full access and access management on the team project
#   │   └── Engineers  → create and update, no delete or access management
#   └── Contractors  → label experiments and datasets on one project; no logs or prompts
#
# Add a team by adding an entry to var.teams.
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
# Variables: everyone must already be a member of the org.
# =============================================================================

variable "admin_emails" {
  type    = list(string)
  default = ["cto@example.com"]
}

variable "auditor_emails" {
  type    = list(string)
  default = ["compliance@example.com"]
}

variable "teams" {
  description = "Team name to the emails of its leads and engineers"
  type = map(object({
    leads     = list(string)
    engineers = list(string)
  }))
  default = {
    Search = { leads = ["search-lead@example.com"], engineers = ["search-eng@example.com"] }
    Chat   = { leads = ["chat-lead@example.com"], engineers = ["chat-eng@example.com"] }
  }
}

variable "contractor_emails" {
  description = "External labelers. They get access to the labeling_team project only."
  type        = list(string)
  default     = ["labeler@vendor.example.com"]
}

variable "labeling_team" {
  description = "Team whose project contractors can label"
  type        = string
  default     = "Chat"
}

locals {
  all_emails = toset(concat(
    var.admin_emails,
    var.auditor_emails,
    var.contractor_emails,
    flatten([for t in values(var.teams) : concat(t.leads, t.engineers)]),
  ))
  user_ids = { for u in data.braintrust_users.all.users : u.email => u.id }
  org_id   = braintrust_group.admins.org_id
}

# One lookup for every email. Emails without a Braintrust account are left out
# of `users`; the postcondition turns that into an error since every email here
# is expected to belong to a user.
data "braintrust_users" "all" {
  emails = local.all_emails

  lifecycle {
    postcondition {
      condition     = length(self.users) == length(local.all_emails)
      error_message = "No Braintrust user for: ${join(", ", setsubtract(local.all_emails, self.users[*].email))}"
    }
  }
}

# =============================================================================
# 1. Team projects, grouped so org-wide roles can be granted once
# =============================================================================

resource "braintrust_project" "team" {
  for_each    = var.teams
  name        = each.key
  description = "Owned by the ${each.key} team"
}

resource "braintrust_project_group" "teams" {
  org_id      = local.org_id
  name        = "Team projects"
  description = "Every team-owned project"
}

resource "braintrust_project_group_membership" "team" {
  for_each         = braintrust_project.team
  project_group_id = braintrust_project_group.teams.id
  project_id       = each.value.id
}

# =============================================================================
# 2. Roles
# =============================================================================

data "braintrust_role" "owner" {
  name = "Owner"
}

resource "braintrust_role" "lead" {
  name        = "Project Lead"
  description = "Full access to a project, including managing who else has access."

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

resource "braintrust_role" "engineer" {
  name        = "Project Engineer"
  description = "Create and update experiments, datasets, prompts, and logs. No delete or access management."

  member_permissions = [
    { permission = "read" },
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

resource "braintrust_role" "auditor" {
  name        = "Auditor"
  description = "Read everything, including who has access. Cannot change anything."

  member_permissions = [
    { permission = "read" },
    { permission = "read_acls" },
  ]
}

# Permissions restricted to object types: no access to logs (may contain PII)
# or prompts (IP).
resource "braintrust_role" "labeler" {
  name        = "External Labeler"
  description = "Open the project, read datasets and experiments, and update experiment scores."

  member_permissions = [
    { permission = "read", restrict_object_type = "project" },
    { permission = "read", restrict_object_type = "dataset" },
    { permission = "read", restrict_object_type = "experiment" },
    { permission = "update", restrict_object_type = "experiment" },
  ]
}

# =============================================================================
# 3. Groups
# =============================================================================

resource "braintrust_group" "admins" {
  name         = "Admins"
  member_users = [for e in var.admin_emails : local.user_ids[e]]
}

resource "braintrust_group" "auditors" {
  name         = "Auditors"
  member_users = [for e in var.auditor_emails : local.user_ids[e]]
}

resource "braintrust_group" "contractors" {
  name         = "Contractors"
  description  = "External labeling vendor"
  member_users = [for e in var.contractor_emails : local.user_ids[e]]
}

resource "braintrust_group" "leads" {
  for_each     = var.teams
  name         = "${each.key} leads"
  member_users = [for e in each.value.leads : local.user_ids[e]]
}

resource "braintrust_group" "engineers" {
  for_each     = var.teams
  name         = "${each.key} engineers"
  member_users = [for e in each.value.engineers : local.user_ids[e]]
}

# =============================================================================
# 4. ACLs: grant each group a role on an object
# =============================================================================

resource "braintrust_acl" "admins" {
  object_type = "organization"
  object_id   = local.org_id
  group_id    = braintrust_group.admins.id
  role_id     = data.braintrust_role.owner.id
}

# project_group_projects covers every project in the group. (An ACL on
# object_type = "project_group" only grants access to the group itself.)
resource "braintrust_acl" "auditors" {
  object_type = "project_group_projects"
  object_id   = braintrust_project_group.teams.id
  group_id    = braintrust_group.auditors.id
  role_id     = braintrust_role.auditor.id
}

resource "braintrust_acl" "leads" {
  for_each    = var.teams
  object_type = "project"
  object_id   = braintrust_project.team[each.key].id
  group_id    = braintrust_group.leads[each.key].id
  role_id     = braintrust_role.lead.id
}

resource "braintrust_acl" "engineers" {
  for_each    = var.teams
  object_type = "project"
  object_id   = braintrust_project.team[each.key].id
  group_id    = braintrust_group.engineers[each.key].id
  role_id     = braintrust_role.engineer.id
}

resource "braintrust_acl" "contractors" {
  object_type = "project"
  object_id   = braintrust_project.team[var.labeling_team].id
  group_id    = braintrust_group.contractors.id
  role_id     = braintrust_role.labeler.id
}

output "project_ids" {
  value = { for k, p in braintrust_project.team : k => p.id }
}
