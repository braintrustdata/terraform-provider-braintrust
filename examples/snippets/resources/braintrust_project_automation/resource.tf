resource "braintrust_project" "my_project" {
  name = "my-project"
}

# Alert when log scores drop below a threshold — send to a webhook.
resource "braintrust_project_automation" "score_alert" {
  project_id  = braintrust_project.my_project.id
  name        = "Low Score Alert"
  description = "Fire when accuracy drops below 0.8"

  logs_config = {
    btql_filter      = "scores.accuracy < 0.8"
    interval_seconds = 300

    webhook_action = {
      url = "https://hooks.example.com/braintrust-alerts"
    }
  }
}

# Data retention policy — keep project logs for 90 days.
resource "braintrust_project_automation" "log_retention" {
  project_id = braintrust_project.my_project.id
  name       = "Log Retention 90d"

  retention_config = {
    object_type    = "project_logs"
    retention_days = 90
  }
}

# Nightly export of log traces to S3.
resource "braintrust_project_automation" "nightly_export" {
  project_id  = braintrust_project.my_project.id
  name        = "Nightly Trace Export"
  description = "Export all traces to S3 every 24 hours"

  export_config = {
    export_type      = "log_traces"
    export_path      = "s3://my-data-lake/braintrust/traces"
    format           = "parquet"
    interval_seconds = 86400

    aws_iam_credentials = {
      role_arn    = "arn:aws:iam::123456789012:role/BraintrustExportRole"
      external_id = "braintrust-export" # must match the role's trust policy
    }
  }
}

# Notify a Slack channel when an environment is updated.
resource "braintrust_project_automation" "env_notify" {
  project_id = braintrust_project.my_project.id
  name       = "Env Update Notification"

  environment_update_config = {
    environment_filter = ["production"]

    slack_action = {
      workspace_id = "T0123456789"
      channel      = "C0123456789"
    }
  }
}
