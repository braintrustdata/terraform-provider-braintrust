variable "org_id" {
  type = string
}

data "braintrust_project_group" "team" {
  org_id = var.org_id
  name   = "Application team"
}
