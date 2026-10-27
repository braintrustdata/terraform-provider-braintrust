# Look up an existing Braintrust project by ID
data "braintrust_project" "by_id" {
  id = "00000000-0000-0000-0000-000000000000" # replace with your project UUID
}

# Or look up by name
data "braintrust_project" "by_name" {
  name = "my-ml-project"
}

output "project_name" {
  value = data.braintrust_project.by_name.name
}
