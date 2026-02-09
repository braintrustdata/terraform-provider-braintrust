# Create a dataset within a project
resource "braintrust_dataset" "evaluation_data" {
  project_id  = braintrust_project.my_project.id
  name        = "evaluation-data"
  description = "Golden dataset for model evaluation"
}
