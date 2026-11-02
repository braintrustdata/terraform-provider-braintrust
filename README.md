# Terraform Provider for Braintrust

A [Terraform](https://www.terraform.io/) provider for managing [Braintrust](https://www.braintrustdata.com/) resources as code.

Detailed docs can be found in the [provider registry](https://registry.terraform.io/providers/braintrustdata/braintrust/latest/docs)

## Quickstart

- [install terraform](https://developer.hashicorp.com/terraform/downloads)
- manually create a Braintrust org
- in your org, set up a Braintrust service key `Settings → Service tokens, click + Service token`
- set up initial terraform file, review with `terraform plan` and create with `terraform apply`

```hcl
terraform {
  required_version = ">= 1.11.1"

  required_providers {
    braintrust = {
      source  = "braintrustdata/braintrust"
      # version = "0.0.0" <--- pin to latest version
    }
  }
}

# Reads the key from the BRAINTRUST_API_KEY environment variable.
provider "braintrust" {
  # can be configured with envars:
  #  BRAINTRUST_API_URL
  #  BRAINTRUST_API_KEY
  # *or* use these settings (settings win over envars)
  api_url = "https://api.braintrust.dev" # <-- the default
  # api_key = "sk-..." <-- recommended to use envar instead
}

resource "braintrust_project" "hello" {
  name        = "hello-terraform"
  description = "Created by the Braintrust Terraform provider"
}

output "project_id" {
  value = braintrust_project.hello.id
}
```

## Examples

[`examples`](examples) has full configurations to copy from:

- [`hello-world`](examples/hello-world): a basic project.
- [`kitchen-sink`](examples/kitchen-sink): a complex project with many resources.
- [`ci-pipeline`](examples/ci-pipeline): per-environment projects, CI service tokens, and regression alerts.
- [`access-control`](examples/access-control): roles, groups, and ACLs for a multi-team org.
- [`ephemeral-service-token`](examples/ephemeral-service-token): create and store a token without persisting its key in Terraform, then revoke it and delete the demo store entry on destroy. Supported by `make run-example`.

To try one against a real Braintrust from a clone of this repo, run:

```sh
export BRAINTRUST_API_KEY="..."
export BRAINTRUST_API_URL="http://localhost:8000"   # optional, defaults to https://api.braintrust.dev
make run-example examples/hello-world                      # plan only, creates nothing
make run-example MODE=apply examples/hello-world           # create the resources and keep them
make run-example MODE=destroy examples/hello-world         # remove them again
make run-example MODE=apply-then-destroy examples/hello-world
```

Each run builds the provider from source. `apply` and `apply-then-destroy` also check that a final plan shows no changes. Terraform state is kept in `.example-state/<api-host>/<name>/` (gitignored) between runs and is tied to the `BRAINTRUST_API_URL` it was created with. Example variables are filled in for you: emails resolve to a member of the key's org, the ephemeral demo's service account defaults to the key's account, and API keys and webhook URLs get placeholder values. Set `TF_VAR_<name>` to override any of them. The ephemeral demo keeps ownership state so destroy can revoke the token before deleting its stored key; failed cleanup preserves files for retry.

`ci-pipeline` and `kitchen-sink` create service tokens, which requires an organization-owner service token. Use one as `BRAINTRUST_API_KEY`, or set `BRAINTRUST_OWNER_SERVICE_TOKEN`.

Examples use fixed resource names and overwrite AI secrets and environment variables with the same name, so **only run them against a dedicated test org**.

## Secrets without state

`braintrust_ai_secret.secret` and `braintrust_env_var.value` are write-only: their values are not stored in resource state. Supply secrets through variables with both `ephemeral = true` and `sensitive = true` so the input variables are also excluded from saved plans. Supply ephemeral inputs again when applying a saved plan; avoid ordinary secret data sources that persist the value upstream.

- Set `secret_version = 1` or `value_version = 1` when supplying a value, and increment the version to update it. Changing only the value does nothing; metadata-only updates do not resend it.
- To adopt an existing resource without rotating its secret, omit both the value and version. `braintrust-terraforming genimport` does this automatically; `generate` emits ephemeral input variables and version `1`.
- To clear an environment variable, increment `value_version` and set `value = null`. Omitting both fields leaves it untouched. AI secrets cannot be cleared through this API.

**Upgrading existing configurations:** keep the input names, add the appropriate version counter, and mark secret input variables ephemeral. These write-only fields can no longer be read as resource outputs. The state upgrade removes stored values without replacing resources; setting the initial version writes the configured value. Apply to persist the upgraded state. Historical state versions, backups, and old saved plans can still contain plaintext; protect them and rotate exposed credentials.

See the [AI secret example](examples/snippets/resources/braintrust_ai_secret/resource.tf) and [environment variable example](examples/snippets/resources/braintrust_env_var/resource.tf).

The managed `resource "braintrust_service_token"` still persists its `key` in state. For state-free generation, use `ephemeral "braintrust_service_token"` and deliver its key through an ephemeral-capable provisioner or a secret-store write-only argument. The [local file-store example](examples/ephemeral-service-token) demonstrates apply-only generation and the full lifecycle: a separate Terraform owner revokes the token and deletes its stored key on destroy. The ephemeral resource itself does not revoke delivered tokens on close.

## Adopting Existing Orgs

If you already have a Braintrust organization, the `braintrust-terraforming` CLI exports it as Terraform config, along with `import` blocks so Terraform adopts the existing resources instead of creating new ones.

```sh
# Install the CLI
curl -fsSL 'https://raw.githubusercontent.com/braintrustdata/terraform-provider-braintrust/refs/heads/main/scripts/install-terraforming.sh' | bash

export BRAINTRUST_API_KEY="sk-..."

# Generate resource config and import blocks
braintrust-terraforming genimport --resource-types all -o braintrust.tf
```

Review `braintrust.tf` and delete any resources you don't want Terraform to manage; delete the matching `import` block too. Then run:

```sh
terraform init
terraform plan   # check for imports, not creates or destroys
terraform apply
```

Useful flags:

- `--include-acls`: also export direct ACLs, which `--resource-types all` leaves out.
- `--projects`: limit the export to specific project names or IDs.
- `--org-id` (or `BRAINTRUST_ORG_ID`): required if your key can access more than one organization.

`generate` writes only the resource config, and `import` writes only the import blocks. Applying `generate` output without import blocks creates duplicate resources.
