# Grant a group a role on a project
resource "braintrust_acl" "ml_team_project_access" {
  object_type = "project"
  object_id   = braintrust_project.my_project.id
  group_id    = braintrust_group.ml_team.id
  role_id     = data.braintrust_role.engineer.id
}

# Grant a specific permission to a group
resource "braintrust_acl" "ml_team_read_logs" {
  object_type = "project"
  object_id   = braintrust_project.my_project.id
  group_id    = braintrust_group.ml_team.id
  permission  = "read"
  restrict_object_type = "project_log"
}
