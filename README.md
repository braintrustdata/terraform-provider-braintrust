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

To try one against a real Braintrust from a clone of this repo, run:

```sh
export BRAINTRUST_API_KEY="..."
export BRAINTRUST_API_URL="http://localhost:8000"   # optional, defaults to https://api.braintrust.dev
make run-example examples/hello-world                      # plan only, creates nothing
make run-example MODE=apply examples/hello-world           # create the resources and keep them
make run-example MODE=destroy examples/hello-world         # remove them again
make run-example MODE=apply-then-destroy examples/hello-world
```

Each run builds the provider from source. `apply` and `apply-then-destroy` also check that a second plan shows no changes. Terraform state is kept in `.example-state/<name>/` (gitignored) between runs and is tied to the `BRAINTRUST_API_URL` it was created with. Example variables are filled in for you: emails resolve to a member of the key's org, and API keys and webhook URLs get placeholder values. Set `TF_VAR_<name>` to override any of them.

`ci-pipeline` and `kitchen-sink` create service tokens, which requires an organization-owner service token. Use one as `BRAINTRUST_API_KEY`, or set `BRAINTRUST_OWNER_SERVICE_TOKEN`.

Examples use fixed resource names and overwrite AI secrets and environment variables with the same name, so **only run them against a dedicated test org**.

## Adopting Existing Orgs

If you already have a Braintrust organization, the `braintrust-terraforming` CLI exports it as Terraform config, along with `import` blocks so Terraform adopts the existing resources instead of creating new ones. Import blocks require Terraform 1.5+.

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
