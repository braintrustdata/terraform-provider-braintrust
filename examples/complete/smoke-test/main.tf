# =============================================================================
# Smoke Test — Full User/Group/ACL Chain
# =============================================================================
#
# A minimal but complete test that exercises every resource type in the
# provider, including user lookup, group membership, custom roles, and ACLs.
#
# Usage:
#   export BRAINTRUST_API_KEY="your-key"
#   terraform apply -var="my_email=you@company.com"
#
# When done:
#   terraform destroy -var="my_email=you@company.com"
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

# =============================================================================
# Variables — just your email.
# =============================================================================

variable "my_email" {
  description = "Your email address (must already be a member of the org)"
  type        = string
}

# =============================================================================
# 1. User lookup — tests data source.
# =============================================================================

data "braintrust_user" "me" {
  email = var.my_email
}

output "user_lookup" {
  value = {
    id    = data.braintrust_user.me.id
    email = data.braintrust_user.me.email
  }
}

# =============================================================================
# 2. Project — the container for everything.
# =============================================================================

resource "braintrust_project" "smoke" {
  name        = "Smoke Test"
  description = "Temporary project for testing the Terraform provider"
}

# =============================================================================
# 3. Custom role — tests role with multiple permissions.
# =============================================================================

resource "braintrust_role" "tester" {
  name        = "Smoke Test Role"
  description = "Custom role for smoke testing: read + create experiments"

  member_permissions = [
    { permission = "read" },
    { permission = "create", restrict_object_type = "experiment" },
    { permission = "update", restrict_object_type = "experiment" },
  ]
}

# =============================================================================
# 4. Look up a built-in role — tests role data source.
# =============================================================================

data "braintrust_role" "viewer" {
  name = "Viewer"
}

output "builtin_role" {
  value = {
    id   = data.braintrust_role.viewer.id
    name = data.braintrust_role.viewer.name
  }
}

# =============================================================================
# 5. Groups — one with you in it, one empty.
# =============================================================================

resource "braintrust_group" "team" {
  name         = "Smoke Test Team"
  description  = "Group with a real user in it"
  member_users = [data.braintrust_user.me.id]
}

resource "braintrust_group" "empty" {
  name        = "Smoke Test Empty Group"
  description = "Empty group to verify groups work without members"
}

# =============================================================================
# 6. ACLs — tests both role-based and permission-based.
# =============================================================================

# Role-based ACL: team gets the custom role on the project.
resource "braintrust_acl" "team_custom_role" {
  object_type = "project"
  object_id   = braintrust_project.smoke.id
  group_id    = braintrust_group.team.id
  role_id     = braintrust_role.tester.id
}

# Built-in role ACL: team also gets the built-in Viewer role.
resource "braintrust_acl" "team_viewer" {
  object_type = "project"
  object_id   = braintrust_project.smoke.id
  group_id    = braintrust_group.team.id
  role_id     = data.braintrust_role.viewer.id
}

# Permission-based ACL (no role): empty group gets direct read permission.
resource "braintrust_acl" "empty_read" {
  object_type = "project"
  object_id   = braintrust_project.smoke.id
  group_id    = braintrust_group.empty.id
  permission  = "read"
}

# =============================================================================
# 7. Project scores — slider and categorical.
# =============================================================================

resource "braintrust_project_score" "quality" {
  project_id  = braintrust_project.smoke.id
  name        = "Quality"
  score_type  = "slider"
  description = "Overall quality score"
}

resource "braintrust_project_score" "safety" {
  project_id  = braintrust_project.smoke.id
  name        = "Safety"
  score_type  = "categorical"
  description = "Content safety classification"
}

# =============================================================================
# 8. Tags.
# =============================================================================

resource "braintrust_project_tag" "smoke" {
  project_id  = braintrust_project.smoke.id
  name        = "smoke-test"
  description = "Tag for smoke test runs"
}

# =============================================================================
# 9. Dataset.
# =============================================================================

resource "braintrust_dataset" "test_cases" {
  project_id  = braintrust_project.smoke.id
  name        = "Test Cases"
  description = "Smoke test dataset"
}

# =============================================================================
# 10. Automation — webhook + retention.
# =============================================================================

resource "braintrust_project_automation" "alert" {
  project_id = braintrust_project.smoke.id
  name       = "Smoke Test Alert"

  logs_config = {
    btql_filter      = "scores.quality < 0.5"
    interval_seconds = 3600

    webhook_action = {
      url = "https://httpbin.org/post"
    }
  }
}

resource "braintrust_project_automation" "retention" {
  project_id = braintrust_project.smoke.id
  name       = "Smoke Test Retention"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 7
  }
}

# =============================================================================
# Summary output.
# =============================================================================

output "summary" {
  value = {
    project_id        = braintrust_project.smoke.id
    role_id           = braintrust_role.tester.id
    group_with_user   = braintrust_group.team.id
    group_empty       = braintrust_group.empty.id
    acl_custom_role   = braintrust_acl.team_custom_role.id
    acl_builtin_role  = braintrust_acl.team_viewer.id
    acl_permission    = braintrust_acl.empty_read.id
    score_slider      = braintrust_project_score.quality.id
    score_categorical = braintrust_project_score.safety.id
    tag               = braintrust_project_tag.smoke.id
    dataset           = braintrust_dataset.test_cases.id
  }
}
