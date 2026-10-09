# A saved project-log view and a dashboard with a chart share the same API resource.
resource "braintrust_project" "app" {
  name = "my-ml-project"
}

resource "braintrust_view" "errors" {
  object_type = "project"
  object_id   = braintrust_project.app.id
  view_type   = "logs"
  name        = "Error traces"

  view_data_json = jsonencode({
    search = { filter = [] }
  })
}

resource "braintrust_view" "health" {
  object_type = "project"
  object_id   = braintrust_project.app.id
  view_type   = "monitor"
  name        = "Project health"

  options_json = jsonencode({
    viewType = "monitor"
    options  = { spanType = "range", rangeValue = "7d" }
  })
  view_data_json = jsonencode({
    custom_charts = {
      version = "0.0.0"
      layout  = { type = "linear", order = ["spans", "note"] }
      charts = {
        spans = {
          title    = "Total spans"
          chartSql = format("SELECT count(1) AS spans FROM project_logs('%s')", braintrust_project.app.id)
          viz      = { type = "singleValue", unitType = "count" }
        }
        note = { type = "note", markdown = "## Project health" }
      }
    }
  })
}
