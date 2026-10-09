resource "braintrust_project_group_membership" "app" {
  project_group_id = braintrust_project_group.team.id
  project_id       = braintrust_project.app.id
}
