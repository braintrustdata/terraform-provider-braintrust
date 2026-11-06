# This lookup returns metadata only, never the raw API key.
data "braintrust_api_key" "existing" {
  id = "00000000-0000-0000-0000-000000000001"
}

output "api_key_owner" {
  value = data.braintrust_api_key.existing.user_email
}
