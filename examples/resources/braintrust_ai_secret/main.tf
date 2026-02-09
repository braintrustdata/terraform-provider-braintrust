# Use Terraform variables to safely manage sensitive API keys.
variable "openai_api_key" {
  type      = string
  sensitive = true
}

variable "anthropic_api_key" {
  type      = string
  sensitive = true
}

# Organization-level OpenAI credentials.
resource "braintrust_ai_secret" "openai" {
  name   = "OpenAI"
  type   = "openai"
  secret = var.openai_api_key
}

# Organization-level Anthropic credentials.
resource "braintrust_ai_secret" "anthropic" {
  name   = "Anthropic"
  type   = "anthropic"
  secret = var.anthropic_api_key
}
