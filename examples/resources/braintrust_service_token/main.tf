# Create a service token for programmatic API access.
resource "braintrust_service_token" "ci" {
  name               = "ci-pipeline"
  service_account_id = "your-service-account-uuid"
}

# The raw token is only available at creation time.
# Store it securely (e.g. in a secrets manager).
output "ci_token" {
  value     = braintrust_service_token.ci.key
  sensitive = true
}
