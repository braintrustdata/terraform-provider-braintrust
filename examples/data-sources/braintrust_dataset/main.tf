# Look up an existing dataset by ID
data "braintrust_dataset" "existing" {
  id = "dataset-uuid-here"
}

output "dataset_name" {
  value = data.braintrust_dataset.existing.name
}
