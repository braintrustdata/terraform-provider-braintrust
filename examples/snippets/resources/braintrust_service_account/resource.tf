# Create a service account for CI automation. To mint tokens for it, see
# braintrust_service_token.
resource "braintrust_service_account" "ci_runner" {
  name = "ci-runner"
}
