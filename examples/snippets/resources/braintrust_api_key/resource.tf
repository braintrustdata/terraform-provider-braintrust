# Authenticate the provider with an unrestricted user API key.
# The new key belongs to that user and defaults to a scoped caller's organization.
# With an unscoped user credential, set org_id to an organization you belong to.
resource "braintrust_api_key" "automation" {
  name               = "terraform-automation"
  expires_in_seconds = 3600
}

# The API may cap the lifetime to the caller's remaining lifetime.
output "api_key_expiration" {
  value = braintrust_api_key.automation.expires_at
}

# Raw keys are returned only at creation. Protect Terraform state and outputs.
output "api_key" {
  value     = braintrust_api_key.automation.key
  sensitive = true
}
