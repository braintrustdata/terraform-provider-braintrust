# For projects managed by Terraform, use separate membership resources. Their
# references ensure membership is removed before either the project or group.
resource "braintrust_project" "app" {
  name = "app"
}

resource "braintrust_project_group" "team" {
  org_id      = "your-organization-id"
  name        = "Application team"
  description = "Projects owned by the application team"
}

resource "braintrust_project_group_membership" "app" {
  project_group_id = braintrust_project_group.team.id
  project_id       = braintrust_project.app.id
}

# A project can also be a member of a second project group.
resource "braintrust_project_group" "shared" {
  org_id = "your-organization-id"
  name   = "Shared projects"
}

resource "braintrust_project_group_membership" "shared_app" {
  project_group_id = braintrust_project_group.shared.id
  project_id       = braintrust_project.app.id
}
