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
