variable "org_id" {
  type = string
}

resource "braintrust_project" "app" {
  name = "app"
}

resource "braintrust_project_group" "team" {
  org_id = var.org_id
  name   = "Application team"
}

resource "braintrust_project_group_membership" "app" {
  project_group_id = braintrust_project_group.team.id
  project_id       = braintrust_project.app.id
}
