# Use Terraform variables to safely manage sensitive API keys.
variable "openai_api_key" {
  type      = string
  sensitive = true
}

variable "anthropic_api_key" {
  type      = string
  sensitive = true
}

# Organization-level AI provider credentials.
# These appear under Organization > "AI providers" in the Braintrust UI
# and are available to all projects in the org.
resource "braintrust_ai_secret" "openai" {
  name   = "OpenAI"
  type   = "openai"
  secret = var.openai_api_key
}

resource "braintrust_ai_secret" "anthropic" {
  name   = "Anthropic"
  type   = "anthropic"
  secret = var.anthropic_api_key
}

variable "azure_api_key" {
  type      = string
  sensitive = true
}

# Azure OpenAI endpoint and deployment settings.
resource "braintrust_ai_secret" "azure" {
  name   = "Azure OpenAI"
  type   = "azure"
  secret = var.azure_api_key
  metadata = jsonencode({
    api_base    = "https://example.openai.azure.com"
    api_version = "2024-10-21"
    deployment  = "my-gpt-deployment"
  })
}

# AI-secret PATCH merges metadata. Removing this attribute, or an existing
# object key, is unsupported. Explicitly replace the resource to clear it.

# For project-level AI provider credentials (visible under Project Settings >
# "Project AI providers"), use braintrust_env_var with secret_type instead:
#
# resource "braintrust_env_var" "project_openai" {
#   object_type = "project"
#   object_id   = braintrust_project.my_project.id
#   name        = "OpenAI"
#   secret_type = "openai"
#   value       = var.openai_api_key
# }
