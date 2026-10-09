# Create a slider-type score for manual labeling.
resource "braintrust_project_score" "accuracy" {
  project_id  = braintrust_project.my_project.id
  name        = "accuracy"
  description = "Manual accuracy rating (0-1)"
  score_type  = "slider"
}

# Create a categorical score.
resource "braintrust_project_score" "quality" {
  project_id  = braintrust_project.my_project.id
  name        = "quality"
  description = "Response quality assessment"
  score_type  = "categorical"

  categories_json = jsonencode([
    { name = "Low", value = 0 },
    { name = "High", value = 1 },
  ])
  config_json = jsonencode({
    multi_select = false
    object_types = ["project_logs"]
  })
}
