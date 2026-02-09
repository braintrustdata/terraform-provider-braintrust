# Create an API key for CI/CD usage.
resource "braintrust_api_key" "ci" {
  name = "ci-pipeline"
}

# The raw key is available as a sensitive output.
output "ci_api_key" {
  value     = braintrust_api_key.ci.key
  sensitive = true
}
