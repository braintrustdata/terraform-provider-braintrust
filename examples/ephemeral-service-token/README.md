# Ephemeral service token with a local secret store

NOTE: the "secret store" used in this example is a simple shell script used to demonstrate the concept. Replace with a real secret service when setting up real infrastructure.

Create a token, save its key outside Terraform, reuse it, then revoke it and delete the stored key on destroy. Requires Terraform 1.11.1+, the Braintrust provider, a POSIX shell, `curl`, `jq`, and standard utilities—no secret-store service or additional provider. Use an existing service account and an organization-owner service token in a **dedicated test org**.


## Run from a checkout

From the repository root, with the tools in `mise.toml` on PATH (`mise exec -- sh` can provide a shell):

```sh
export BRAINTRUST_API_KEY="your-organization-owner-service-token"
# Optional: defaults to the service account associated with the API key.
# export TF_VAR_service_account_id="your-existing-service-account-id"
# Optional: defaults to production; prefer a dedicated local stack.
# export BRAINTRUST_API_URL="http://localhost:8000"

make run-example MODE=plan examples/ephemeral-service-token
make run-example MODE=apply examples/ephemeral-service-token
make run-example MODE=plan examples/ephemeral-service-token
make run-example MODE=destroy examples/ephemeral-service-token
```

- **Plan:** creates no token.
- **Apply:** creates one token and saves its key. Repeat plans/applies reuse the entry.
- **Destroy:** revokes the token first, then deletes the entry and its non-secret receipt. Failed revocation leaves the files and ownership state for retry.
- **Apply after destroy:** creates a new token.

The runner builds the provider and copies this example into `.example-state/<api-host>/ephemeral-service-token/`. For `http://localhost:8000`, the store is `.example-state/localhost_8000/ephemeral-service-token/.secrets/service-token`. No `terraform init` is needed. `MODE=apply-then-destroy` exercises creation and cleanup in one run.

**Already ran the previous version of this demo?** Run `MODE=apply` once before `MODE=destroy`. Apply registers ownership of the existing file without creating a replacement token or changing its key. Destroy can then revoke it and remove the file.

Tokens default to a one-hour lifetime; override `TF_VAR_expires_in_seconds` if that exceeds the caller token's remaining lifetime. Keep the API key available for destroy. Keep each working directory tied to one API URL and service account. When using a published provider directly, copy `main.tf` and `store.sh`, set `TF_VAR_service_account_id` and `TF_VAR_api_url`, and run `terraform init`, `plan`, `apply`, and `destroy` normally.

## What is managed

The fake store is `.secrets/service-token`: token ID on the first line, raw key on the second. The directory is mode `0700`, the file is `0600`, and both it and the receipt are gitignored. **This is plaintext demo storage, not a production secret manager.** Privileged local users can read the file or process environment.

Two built-in Terraform resources provide the lifecycle:

- `terraform_data.token` records non-secret paths, the API URL, and an ownership UUID. Its destroy provisioner calls the Braintrust delete API and removes the files only after confirmed revocation or an already-missing response.
- `terraform_data.delivery` passes the ephemeral ID and key to `store.sh` through environment variables. The script records the ID in `.service-token-id` before saving the key atomically. Keeping cleanup on a separate resource means a failed, tainted delivery does not disable revocation.

Neither the token key nor the admin API key goes into Terraform inputs, outputs, state, or saved plans. The receipt contains only the token ID. It allows cleanup even if key delivery fails or the key file is lost.

## Why plans and repeat applies do not mint tokens

`fileexists` checks for the entry or receipt without reading the key into Terraform. Either file disables generation. The required `nonce` is set to `timestamp()`, which is unknown during planning, so generation waits until apply. Do not replace it with a constant: known inputs can generate tokens during planning.

Both lifecycle resources remain in state after apply. The runner checks convergence without removing them. The ephemeral resource itself still does not revoke tokens on close; **this example's destroy provisioner implements revocation**.

For a production store, use its provider's write-only input for delivery and an ephemeral read when Terraform needs the key. Ordinary secret data sources or `local_sensitive_file.content` can put the key back into state. This shell-based store demonstrates ownership, not a universal secret-store integration.

## Failures and limits

- On revocation failure, fix credentials/connectivity and retry destroy. A generic permission error does not count as successful cleanup. Already-missing responses use the same 404 / explicit 400-or-403 “does not exist” semantics as the provider.
- On delivery failure, run destroy before retrying apply. The ID receipt normally lets it revoke the token even though delivery failed. A crash can leave private `.service-token.*` files; the runner preserves them for inspection instead of deleting unrecognized files.
- If only the key is lost, the receipt prevents accidental reissuance and still supports destroy. It cannot recover the original key; destroy and apply to obtain a new one.
- If both the receipt and entry are lost, or execution stops before the receipt is written, **do not blindly apply again**. Inspect Braintrust for a token named `token_name` (default `my-ci-token`); while it exists, apply refuses to mint another with the same name. Revoke orphaned tokens before removing ownership state or recovery files. An uncertain create response may also require reconciliation; follow the provider's diagnostic.
- The store checks existence, not token validity or expiry. It does not implement automatic rotation, account changes, or remote drift detection. Run only one writer, and re-plan if files change between plan and apply.
- Keep the Terraform blocks and `store.sh` present until destroy finishes. Removing a block, tainting the owner, or losing state can bypass Terraform destroy-time provisioners; this is a demo, not a production lifecycle resource.
