# Create a basic Braintrust project
resource "braintrust_project" "example" {
  name        = "my-ml-project"
  description = "A project for managing ML experiments"
}

# Create a project with custom settings
resource "braintrust_project" "advanced" {
  name                   = "advanced-project"
  description            = "A project with custom comparison settings"
  comparison_key         = "input"
  baseline_experiment_id = "00000000-0000-0000-0000-000000000000"
}
