# Create a project tag for labeling experiments.
resource "braintrust_project_tag" "correctness" {
  project_id  = braintrust_project.my_project.id
  name        = "correctness"
  description = "Marks experiments focused on correctness evaluation"
  color       = "#4CAF50"
}

# A simple tag with just a name and color.
resource "braintrust_project_tag" "staging" {
  project_id = braintrust_project.my_project.id
  name       = "staging"
  color      = "#FF9800"
}
