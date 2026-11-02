terraform {
  required_version = ">= 1.11.1"
}

# Increment value_version whenever you change value; changing only value does
# nothing. Version counters are non-ephemeral. A new version with a null or
# omitted value explicitly clears it; omitting both leaves it untouched on update.
# Ephemeral sensitive variables keep inputs out of plans and state.

variable "org_id" {
  type = string
}

resource "braintrust_project" "my_project" {
  name = "my-project"
}

variable "openai_api_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

# A regular environment variable scoped to a project.
resource "braintrust_env_var" "api_base_url" {
  object_type   = "project"
  object_id     = braintrust_project.my_project.id
  name          = "API_BASE_URL"
  value         = "https://api.example.com"
  value_version = 1
}

# An environment variable available to every project in the organization.
resource "braintrust_env_var" "log_level" {
  object_type   = "organization"
  object_id     = var.org_id
  name          = "LOG_LEVEL"
  value         = "info"
  value_version = 1
}

# A project-level AI provider credential.
# Setting secret_type to the provider name (e.g. "openai") causes this
# to appear under "Project AI providers" in the Braintrust UI.
resource "braintrust_env_var" "project_openai" {
  object_type   = "project"
  object_id     = braintrust_project.my_project.id
  name          = "OpenAI"
  secret_type   = "openai"
  value         = var.openai_api_key
  value_version = 1
}

# For organization-level AI providers, use braintrust_ai_secret instead.
