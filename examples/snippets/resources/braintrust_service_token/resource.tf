# Creating service tokens requires an organization-owner service token. If the
# rest of the configuration uses a user API key, add an aliased provider.
variable "owner_service_token" {
  type      = string
  sensitive = true
}

provider "braintrust" {
  alias   = "owner"
  api_key = var.owner_service_token
}

resource "braintrust_service_account" "ci" {
  name = "ci-pipeline"
}

# Create a service token for programmatic API access. Tokens cannot be
# changed in place; to rotate one, replace it with
# `terraform apply -replace=braintrust_service_token.ci`.
resource "braintrust_service_token" "ci" {
  provider           = braintrust.owner
  name               = "ci-pipeline"
  service_account_id = braintrust_service_account.ci.id
}

# The raw token is only available at creation time.
# Store it securely (e.g. in a secrets manager).
output "ci_token" {
  value     = braintrust_service_token.ci.key
  sensitive = true
}
