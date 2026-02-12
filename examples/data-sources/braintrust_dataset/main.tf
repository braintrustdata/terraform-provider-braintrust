# Look up an existing dataset by ID
data "braintrust_dataset" "by_id" {
  id = "dataset-uuid-here"
}

# Or look up by name within a project
data "braintrust_dataset" "by_name" {
  project_id = data.braintrust_project.example.id
  name       = "eval-dataset"
}

output "dataset_name" {
  value = data.braintrust_dataset.by_name.name
}
