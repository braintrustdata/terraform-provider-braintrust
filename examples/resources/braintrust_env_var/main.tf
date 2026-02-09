# Manage an environment variable scoped to a project.
resource "braintrust_env_var" "openai_key" {
  object_type = "project"
  object_id   = braintrust_project.my_project.id
  name        = "OPENAI_API_KEY"
  value       = var.openai_api_key
  secret_type = "openai"
}
