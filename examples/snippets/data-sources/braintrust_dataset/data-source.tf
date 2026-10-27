# Look up an existing dataset by ID
data "braintrust_dataset" "by_id" {
  id = "00000000-0000-0000-0000-000000000000" # replace with your dataset UUID
}

# Or look up by name within a project
data "braintrust_dataset" "by_name" {
  project_id = "00000000-0000-0000-0000-000000000000" # replace with your project UUID
  name       = "eval-dataset"
}

output "dataset_name" {
  value = data.braintrust_dataset.by_name.name
}
