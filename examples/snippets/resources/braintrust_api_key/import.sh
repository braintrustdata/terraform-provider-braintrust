# Import manages metadata and revocation, not recovery of the raw secret.
# key is null after import; expires_in_seconds is not inferred from expires_at.
# Before importing, match the existing name and organization and omit
# expires_in_seconds. Keeping the creation example's TTL causes replacement
# on the next apply, revoking the imported key even if its lifetime matched.
# Terraform can revoke the imported key on destroy or replacement.
terraform import braintrust_api_key.automation 00000000-0000-0000-0000-000000000001
