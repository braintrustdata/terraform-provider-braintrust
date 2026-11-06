# Requires Terraform 1.11.1 or later. Authenticate the Braintrust provider with an
# unrestricted user API key. Service tokens, OAuth credentials, and restricted
# keys cannot create user API keys. The new key belongs to the caller's user.
# Organization-scoped credentials can mint keys only in their own organization.
# Unscoped user credentials must supply org_id for an organization they belong to.
ephemeral "braintrust_api_key" "delivery" {
  # A fresh name avoids adopting or overwriting an existing same-name key.
  name = "terraform-delivery-${uuid()}"

  # Unknown until apply: a literal nonce can mint keys during every plan.
  nonce = timestamp()

  # Defaults to the caller's organization for organization-scoped credentials,
  # and must match that organization if supplied. Required for unscoped user
  # credentials: select an organization the caller belongs to.
  # org_id = "00000000-0000-0000-0000-000000000001"

  # Positive lifetime; the API caps it at the caller's remaining lifetime.
  expires_in_seconds = 3600
}

# Deliver ephemeral.braintrust_api_key.delivery.key through an ephemeral-aware
# consumer (for example, a supported write-only secret-store argument). Deliver
# ephemeral.braintrust_api_key.delivery.id with it for later revocation.
# Do not pass the key to ordinary persisted resource arguments or root outputs.
# Terraform does not retain either output in plan/state or revoke the remote key
# when this ephemeral resource closes or the configuration is destroyed.
# Callers own secure delivery, lookup, rotation, expiry, and revocation. If
# delivery fails, inspect/revoke the new key before minting another: the raw key
# cannot be recovered and this resource keeps no recovery ID in state.
