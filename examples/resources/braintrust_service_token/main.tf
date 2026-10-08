# Create a service account first.
resource "braintrust_service_account" "ci" {
  name = "ci-pipeline"
}

# Create a service token for programmatic API access.
resource "braintrust_service_token" "ci" {
  name               = "ci-pipeline"
  service_account_id = braintrust_service_account.ci.id
  expires_in_seconds = 3600
}

# The raw token is only available at creation time.
# Store it securely (e.g. in a secrets manager).
output "ci_token" {
  value     = braintrust_service_token.ci.key
  sensitive = true
}
