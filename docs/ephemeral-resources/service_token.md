---
page_title: "braintrust_service_token Ephemeral Resource - terraform-provider-braintrust"
subcategory: ""
description: |-
  Generates a Braintrust service token without storing its data in Terraform plans or state.
---

# braintrust_service_token (Ephemeral Resource)

Generates a Braintrust service token without storing its data in Terraform plan or state files.

**Ephemeral describes the Terraform data, not the remote token lifetime.** The provider does not revoke the token when Terraform closes the ephemeral resource or destroys the configuration, and does not automatically rotate it. Callers own durable delivery, store lookup, rotation, expiration, and revocation. The raw `key` is returned only at creation and cannot be recovered from the API.

**Set `nonce = timestamp()` (or `uuid()`) so planning does not mint a token.** Terraform opens ephemeral resources during plan when every input is known; those functions are unknown until apply, which defers Open. A literal `nonce` mints a token on every plan. Each Open creates a new token, so use a unique name within the service account. Existing same-name tokens are rejected, never adopted.

Creation requires an organization-owner service token. When the caller token expires, `expires_in_seconds` must be less than its remaining lifetime.

If a create response is lost or incomplete, the provider does not retry creation. It attempts revocation only when the create response identifies the sole newly reconciled matching token. Without a response ID, even a single new name match may belong to another caller and is never automatically revoked; the error identifies any known IDs for manual inspection and revocation. No recovery ID is retained in Terraform state. A successful Open followed by lost downstream delivery cannot be automatically detected or recovered; inspect and revoke that token before minting a replacement.

## Example: dependency-light local secret store

This demonstration needs Terraform 1.11.1+, an existing service account, an organization-owner service token, a POSIX shell, `curl`, `jq`, and standard utilities. No AWS account, Vault server, or additional provider is required. Use a dedicated test org.

The token uses a fixed name with `nonce = timestamp()`. The nonce is unknown during planning, which defers generation until apply. File-existence checks prevent repeat issuance. Persistent `terraform_data` resources own delivery and cleanup: destroy revokes the token before removing the key. Cleanup is separate from delivery so a tainted delivery resource does not skip revocation.

```terraform
terraform {
  required_version = ">= 1.11.1"
  required_providers {
    braintrust = {
      source = "braintrustdata/braintrust"
    }
  }
}

# BRAINTRUST_API_KEY must be an organization-owner service token.
provider "braintrust" {
  api_url = var.api_url
}

variable "api_url" {
  description = "Braintrust API endpoint; the runner fills this from BRAINTRUST_API_URL."
  type        = string
  default     = "https://api.braintrust.dev"
}

variable "service_account_id" {
  description = "Existing service account that will own the generated token."
  type        = string
}

variable "token_name" {
  description = "Name of the generated token. Use a dedicated test service account."
  type        = string
  default     = "my-ci-token"
}

variable "expires_in_seconds" {
  description = "Limit the lifetime of demo tokens; must fit within the caller token's remaining lifetime."
  type        = number
  default     = 3600
}

locals {
  secret_file   = abspath("${path.module}/.secrets/service-token")
  token_id_file = abspath("${path.module}/.service-token-id")
  needs_token   = !fileexists(local.secret_file) && !fileexists(local.token_id_file)
}

# timestamp() is unknown during planning, so Open is deferred until apply.
# Do not replace nonce with a constant: known inputs mint during planning.
ephemeral "braintrust_service_token" "new" {
  count = local.needs_token ? 1 : 0

  name               = var.token_name
  nonce              = timestamp()
  service_account_id = var.service_account_id
  expires_in_seconds = var.expires_in_seconds
}

# The owner survives a failed delivery provisioner, so destroy still runs
# revocation even if Terraform marks the delivery resource as tainted.
# State contains paths, the API URL, and an ownership UUID, never the key.
resource "terraform_data" "token" {
  input = {
    secret_file   = local.secret_file
    token_id_file = local.token_id_file
    store_script  = abspath("${path.module}/store.sh")
    api_url       = var.api_url
  }

  provisioner "local-exec" {
    when    = destroy
    command = "sh \"$STORE_SCRIPT\" destroy"
    environment = {
      STORE_SCRIPT  = self.input.store_script
      SECRET_FILE   = self.input.secret_file
      TOKEN_ID_FILE = self.input.token_id_file
      API_URL       = self.input.api_url
    }
  }
}

# AWS Secrets Manager equivalent of the initial delivery step:
#
# resource "aws_secretsmanager_secret" "token" {
#   name_prefix = "braintrust-demo-"
# }
#
# resource "aws_secretsmanager_secret_version" "token" {
#   secret_id                = aws_secretsmanager_secret.token.id
#   secret_string_wo         = ephemeral.braintrust_service_token.new[0].key
#   secret_string_wo_version = 1 # Increment when deliberately rotating the key.
# }
#
# Use secret_string_wo, NOT secret_string, to keep the key out of AWS resource
# state too. Requires the hashicorp/aws provider with write-only support.
# https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/secretsmanager_secret_version
#
# This shows delivery syntax, not a drop-in replacement: new[0] exists only
# during issuance. A real integration must replace the local existence check,
# handle subsequent applies without reissuing, and retain the non-secret token
# ID for revocation. Deleting the AWS secret does NOT revoke the Braintrust
# token; keep explicit revocation, and account for AWS's recovery window.
# If Terraform needs to read the key, use an ephemeral read, not a data source.
#
resource "terraform_data" "delivery" {
  triggers_replace = [terraform_data.token.id]

  provisioner "local-exec" {
    command = "sh \"$STORE_SCRIPT\" create"
    environment = {
      STORE_SCRIPT  = terraform_data.token.output.store_script
      SECRET_FILE   = terraform_data.token.output.secret_file
      TOKEN_ID_FILE = terraform_data.token.output.token_id_file
      TOKEN_ID      = local.needs_token ? ephemeral.braintrust_service_token.new[0].id : ""
      TOKEN_KEY     = local.needs_token ? ephemeral.braintrust_service_token.new[0].key : ""
    }
  }
}
```

Place the following `store.sh` beside the Terraform file:

```sh
#!/usr/bin/env sh
# Demo-only plaintext storage: first line is the token ID, second is the key.
# Never print the key, place it in command arguments, or overwrite an entry.
set -eu
umask 077

: "${SECRET_FILE:?SECRET_FILE must be set}"
: "${TOKEN_ID_FILE:?TOKEN_ID_FILE must be set}"

directory=${SECRET_FILE%/*}
if [ -L "$directory" ]; then
  echo "Refusing a symlinked secret directory" >&2
  exit 1
fi
if [ -L "$SECRET_FILE" ] || [ -L "$TOKEN_ID_FILE" ]; then
  echo "Refusing a symlinked secret entry" >&2
  exit 1
fi

case "${1:?usage: store.sh create|destroy}" in
destroy)
  : "${BRAINTRUST_API_KEY:?BRAINTRUST_API_KEY must be set to revoke the token}"
  : "${API_URL:?API_URL must be set}"
  # The non-secret receipt survives loss of the key or failed delivery.
  # Fall back to the old demo's entry when adopting an existing store.
  id_file=$TOKEN_ID_FILE
  [ -f "$id_file" ] || id_file=$SECRET_FILE
  if [ ! -f "$id_file" ]; then
    echo "Token receipt missing; reconcile this owner's tokens in Braintrust before removing its Terraform state" >&2
    exit 1
  fi
  IFS= read -r token_id < "$id_file"
  case "$token_id" in
    "" | *[!a-fA-F0-9-]*) echo "Invalid stored token ID; refusing cleanup" >&2; exit 1 ;;
  esac
  [ "${#token_id}" -eq 36 ] || { echo "Invalid stored token ID; refusing cleanup" >&2; exit 1; }
  # Keep credentials out of process arguments and response bodies off disk.
  response=$(printf 'Authorization: Bearer %s\n' "$BRAINTRUST_API_KEY" |
    curl -q --silent --show-error --max-time 30 --header @- \
      --request DELETE --write-out '\n%{http_code}' \
      "${API_URL%/}/v1/service_token/$token_id")
  # curl appends the status on the final line, after the response body.
  status=${response##*'
'}
  case "$status" in
    2?? | 404) ;;
    400 | 403)
      # Match the provider's missing-resource semantics, not arbitrary
      # permission errors. jq slurps the JSON body and appended status.
      if ! printf '%s' "$response" | jq -es '
        (.[0].error // .[0].message // "") | strings |
        ascii_downcase | contains("does not exist")' >/dev/null; then
        echo "Token revocation failed (HTTP $status); store retained for retry" >&2
        exit 1
      fi
      ;;
    *) echo "Token revocation failed (HTTP $status); store retained for retry" >&2; exit 1 ;;
  esac
  rm -f "$SECRET_FILE"
  rm -f "$TOKEN_ID_FILE"
  # Preserve unrelated or interrupted-delivery files.
  rmdir "$directory" 2>/dev/null || true
  exit 0
  ;;
create)
  # Applying the upgraded demo can take ownership of an existing demo entry.
  if [ -f "$SECRET_FILE" ] && [ -z "${TOKEN_ID:-}" ] && [ -z "${TOKEN_KEY:-}" ]; then
    if [ ! -f "$TOKEN_ID_FILE" ]; then
      IFS= read -r token_id < "$SECRET_FILE"
      (set -C; printf '%s\n' "$token_id" > "$TOKEN_ID_FILE")
    fi
    exit 0
  fi
  # A receipt without an entry means an earlier delivery failed after minting.
  if [ -z "${TOKEN_ID:-}" ] && [ -f "$TOKEN_ID_FILE" ]; then
    IFS= read -r token_id < "$TOKEN_ID_FILE"
    echo "Token $token_id was issued but its key was not stored, and the key cannot be recovered. Run destroy to revoke it, then apply again." >&2
    exit 1
  fi
  : "${TOKEN_ID:?TOKEN_ID must be set}"
  : "${TOKEN_KEY:?TOKEN_KEY must be set}"
  # Record the non-secret ID before filesystem operations that might fail.
  # Never overwrite an earlier receipt: reconcile it before retrying.
  (set -C; printf '%s\n' "$TOKEN_ID" > "$TOKEN_ID_FILE")
  ;;
*) echo "usage: store.sh create|destroy" >&2; exit 1 ;;
esac
mkdir -p "$directory"
chmod 700 "$directory"
if [ -e "$SECRET_FILE" ] || [ -L "$SECRET_FILE" ]; then
  echo "Secret entry already exists; refusing to overwrite it" >&2
  exit 1
fi

temporary=$(mktemp "$directory/.service-token.XXXXXX")
trap 'rm -f "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
printf '%s\n%s\n' "$TOKEN_ID" "$TOKEN_KEY" > "$temporary"
chmod 600 "$temporary"
# Publishing a complete file with a hard link is atomic and does not replace
# an existing file if another writer won after the existence check above.
ln "$temporary" "$SECRET_FILE"
```

**Demo only:** `.secrets/service-token` is plaintext, with the token ID on its first line and key on its second. `.service-token-id` is a non-secret receipt written before delivery, allowing cleanup after a lost key or failed delivery. Keep both files out of version control. The script uses owner-only permissions and refuses to overwrite an existing entry. Existence does not prove validity or expiry. Run only one writer and re-plan if files change between plan and apply.

**Cleanup:** Keep `BRAINTRUST_API_KEY` available for destroy. Revocation failure preserves the entry, receipt, and ownership state for retry. If both files are lost, reconcile tokens in Braintrust before applying again. Keep these Terraform blocks and the script present until destroy finishes. This example's destroy provisioner implements revocation; the ephemeral resource itself does not revoke on close. See the [runnable example, upgrade, and recovery instructions](https://github.com/braintrustdata/terraform-provider-braintrust/tree/main/examples/ephemeral-service-token).

For a production secret store, replace local delivery with your chosen provider's write-only argument and use an ephemeral read if Terraform needs to retrieve the key. Do not use ordinary data sources or `local_sensitive_file.content` for the raw key, as they can persist it in state. Adapt the missing-entry check and lifecycle to the store rather than assuming every store supports a nullable lookup.

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `name` (String) Name of the token. Must be unique within its service account for safe creation and recovery.
- `nonce` (String) Set to timestamp() or uuid(). Not sent to the API; its value is unknown during planning, so Terraform defers minting until apply. A literal value mints a token during plan.
- `service_account_id` (String) ID of the existing service account to which this token belongs.

### Optional

- `expires_in_seconds` (Number) Positive token lifetime in seconds. Required when minting with an expiring owner token and must be less than its remaining lifetime. Omit for no expiration when permitted by the API.

### Read-Only

- `id` (String) ID of the created token. Deliver it with the key so the token can later be revoked.
- `key` (String, Sensitive) Raw token, available only at creation. Never stored in Terraform plan or state files; callers must deliver it securely.
