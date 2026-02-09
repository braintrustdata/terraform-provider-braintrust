# A regular environment variable scoped to a project.
resource "braintrust_env_var" "api_base_url" {
  object_type = "project"
  object_id   = braintrust_project.my_project.id
  name        = "API_BASE_URL"
  value       = "https://api.example.com"
}

# A project-level AI provider credential.
# Setting secret_type to the provider name (e.g. "openai") causes this
# to appear under "Project AI providers" in the Braintrust UI.
resource "braintrust_env_var" "project_openai" {
  object_type = "project"
  object_id   = braintrust_project.my_project.id
  name        = "OpenAI"
  secret_type = "openai"
  value       = var.openai_api_key  # use a sensitive variable
}

# For organization-level AI providers, use braintrust_ai_secret instead.
