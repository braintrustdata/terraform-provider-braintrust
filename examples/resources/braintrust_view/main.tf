# A saved project-log view and a dashboard with SQL charts share the same API resource.
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
      # Chart IDs are keys in charts; layout.order controls their display order.
      layout = { type = "linear", order = ["spans", "latency_by_model", "note"] }
      charts = {
        spans = {
          title = "Total spans"
          # The project-scoped dashboard supplies the project to project_logs().
          chartSql = "SELECT count(1) AS spans FROM project_logs()"
          viz      = { type = "singleValue", unitType = "count" }
        }
        latency_by_model = {
          title = "Average latency over time by model"
          # Timeseries charts add time bucketing and the dashboard time range.
          # Select each series label and aggregate, then GROUP BY the label.
          chartSql = <<-SQL
            SELECT
              metadata.model AS model,
              avg(metrics.duration) AS avg_duration
            FROM project_logs()
            WHERE metadata.model IS NOT NULL
            GROUP BY model
          SQL
          columnOptions = {
            model        = { displayName = "Model" }
            avg_duration = { displayName = "Average latency" }
          }
          viz = {
            type              = "timeseries"
            timeseriesVizType = "lines"
            unitType          = "duration"
            targetInterval    = "day"
          }
        }
        note = { type = "note", markdown = "## Project health" }
      }
    }
  })
}
