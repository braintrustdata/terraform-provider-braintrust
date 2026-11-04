# Ephemeral sensitive variables keep inputs out of plans and state;
# older state histories may still contain them.
terraform {
  required_version = ">= 1.11.1"
}

variable "openai_api_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

variable "anthropic_api_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

# Organization-level AI provider credentials.
# These appear under Organization > "AI providers" in the Braintrust UI
# and are available to all projects in the org.
# Increment secret_version whenever you change its secret; changing only the
# secret does nothing. Keep version counters non-ephemeral.
resource "braintrust_ai_secret" "openai" {
  name           = "OpenAI"
  type           = "openai"
  secret         = var.openai_api_key
  secret_version = 1
}

resource "braintrust_ai_secret" "anthropic" {
  name           = "Anthropic"
  type           = "anthropic"
  secret         = var.anthropic_api_key
  secret_version = 1
}

# For project-level AI provider credentials (visible under Project Settings >
# "Project AI providers"), use braintrust_env_var with secret_type instead:
#
# resource "braintrust_env_var" "project_openai" {
#   object_type   = "project"
#   object_id     = braintrust_project.my_project.id
#   name          = "OpenAI"
#   secret_type   = "openai"
#   value         = var.openai_api_key
#   value_version = 1
# }
