# Create a service account for CI automation.
resource "braintrust_service_account" "ci_runner" {
  name = "ci-runner"
}

# Create a token for the service account.
resource "braintrust_service_token" "ci_runner" {
  name               = "ci-runner-token"
  service_account_id = braintrust_service_account.ci_runner.id
}
