# Look up an existing Braintrust project by ID
data "braintrust_project" "example" {
  id = "00000000-0000-0000-0000-000000000000"
}

output "project_name" {
  value = data.braintrust_project.example.name
}

output "project_org_id" {
  value = data.braintrust_project.example.org_id
}
