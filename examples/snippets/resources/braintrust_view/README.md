# SQL-backed dashboard charts

[`resource.tf`](resource.tf) defines two SQL charts inside `braintrust_view.view_data_json.custom_charts`: a big-number span count and a time series grouped by model. Each chart has a stable key in `charts`; the same key appears in `layout.order`. `chartSql` is the query shown in Braintrust's chart editor, while `viz` selects the rendering and unit.

For query syntax and the chart editor's behavior, see the official [Braintrust SQL charts documentation](https://docs.braintrust.dev/observe/dashboards/sql-charts#sql-charts).

The project-scoped dashboard supplies its project to `project_logs()`. For a time series, write the aggregate and `GROUP BY` for every non-aggregate output column. Braintrust supplies the time bucket and applies the dashboard's time range, so the saved `chartSql` does not need its own time column, date filter, ordering, or limit. Replace `metadata.model` with a field your project actually logs.

For a chart like **Estimated cost over time by scenario version** in the UI, the chart entry can be:

```hcl
scenario_cost = {
  title = "Estimated cost over time by scenario version"
  chartSql = <<-SQL
    SELECT
      metadata.scenario_version AS scenario_version,
      avg(metrics.estimated_cost) AS avg_estimated_cost
    FROM project_logs(shape => 'summary')
    WHERE metadata.scenario_version IS NOT NULL
    GROUP BY scenario_version
  SQL
  columnOptions = {
    scenario_version   = { displayName = "Scenario version" }
    avg_estimated_cost = { displayName = "Average estimated cost" }
  }
  viz = {
    type              = "timeseries"
    timeseriesVizType = "lines"
    unitType          = "cost"
    targetInterval    = "day"
  }
}
```

Add `"scenario_cost"` to `layout.order` when adding that entry to `charts`. This query uses trace-level `metrics.estimated_cost`, which is available on the `summary` shape. Braintrust can rewrite that shape when a dashboard filter is applied, so check the chart under the filters your viewers will use before adopting it. Check the field names and query results in the target project; `metadata.scenario_version` is only an example field.
