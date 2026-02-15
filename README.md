# Terraform Provider for Braintrust

A [Terraform](https://www.terraform.io/) provider for managing [Braintrust](https://www.braintrustdata.com/) resources as code.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24 (to build the provider plugin)

## Resources

| Resource | Description |
|---|---|
| `braintrust_project` | Projects for organizing experiments, datasets, and logs |
| `braintrust_dataset` | Datasets belonging to a project |
| `braintrust_prompt` | Prompt definitions belonging to a project |
| `braintrust_group` | User groups for access control |
| `braintrust_role` | Custom roles with fine-grained permissions |
| `braintrust_acl` | Access control list entries (bind groups to roles/permissions on objects) |
| `braintrust_env_var` | Environment variables scoped to org, project, or function |
| `braintrust_ai_secret` | AI provider credentials (OpenAI, Anthropic, etc.) at org level |
| `braintrust_api_key` | API keys for user-level authentication |
| `braintrust_service_account` | Service accounts for non-human automation identities |
| `braintrust_service_token` | Service tokens for programmatic access via service accounts |
| `braintrust_project_score` | Score definitions for labeling experiments |
| `braintrust_project_tag` | Tags for filtering experiments and logs |
| `braintrust_project_automation` | Automation rules (alerts, exports, retention, env notifications) |

## Data Sources

| Data Source | Description |
|---|---|
| `braintrust_project` | Look up a project by ID or name |
| `braintrust_dataset` | Look up a dataset by ID or by name + project |
| `braintrust_prompt` | Look up a prompt by ID or by name + project |
| `braintrust_group` | Look up a group by name |
| `braintrust_role` | Look up a role by name (e.g. built-in `Viewer`, `Admin`) |
| `braintrust_user` | Look up a user by email or ID (useful for group membership) |

## Installation

Add the provider to your `required_providers` block and run `terraform init`:

```hcl
terraform {
  required_providers {
    braintrust = {
      source  = "braintrustdata/braintrust"
      version = "~> 0.1"
    }
  }
}
```

## Authentication

The provider requires a Braintrust API key. You can create one in the Braintrust [organization settings](https://www.braintrustdata.com/app/settings?subroute=api-keys).

Set it via the `BRAINTRUST_API_KEY` environment variable (recommended) or in the provider block.

## Usage

```hcl
provider "braintrust" {
  # api_key = "..."  # or set BRAINTRUST_API_KEY env var
}

resource "braintrust_project" "my_project" {
  name        = "my-ml-project"
  description = "Production ML evaluation project"
}

resource "braintrust_dataset" "eval_data" {
  project_id  = braintrust_project.my_project.id
  name        = "eval-dataset"
  description = "Golden evaluation dataset"
}

# Look up an existing project by name
data "braintrust_project" "existing" {
  name = "my-ml-project"
}
```

### Email-First User Membership

For group membership and ACL user grants, write config using emails and resolve to IDs via
`data "braintrust_user"`:

```hcl
data "braintrust_user" "alice" {
  email = "alice@company.com"
}

resource "braintrust_group" "engineering" {
  name         = "Engineering"
  member_users = [data.braintrust_user.alice.id]
}

resource "braintrust_acl" "alice_project_viewer" {
  object_type = "project"
  object_id   = braintrust_project.my_project.id
  user_id     = data.braintrust_user.alice.id
  permission  = "read"
}
```

## Adopting Existing Orgs

If you already have a Braintrust organization and want an initial Terraform baseline, use
[braintrust-terraforming](https://github.com/braintrustdata/braintrust-terraforming) to generate
resource blocks (and optional import blocks) from your current org state.

```sh
export BRAINTRUST_API_KEY="sk-..."

# Generate resource config
braintrust-terraforming generate --resource-types all -o braintrust.tf

# Or generate config + import blocks together
braintrust-terraforming genimport --resource-types all -o braintrust.tf
```

After generating, review and trim the output to the resources you want to manage, then run
`terraform init` and `terraform apply` to import them into state.

## What To Manage In Terraform

Terraform works best for stable control-plane state, not high-churn application content.

### Good Terraform Fits

- Access and identity controls: groups, roles, ACLs, service accounts, service tokens.
- Environment scaffolding: projects, score definitions, tags, baseline datasets (container objects).
- Organization policy/configuration: secrets wiring, automations, retention policy.

### Better Managed In Code Pipelines

- High-churn product logic (for example: frequently changing prompts/functions).
- Data payloads and runtime events (dataset rows, logs, experiment events).
- Artifacts that ship with application releases and should follow app deployment cadence.

### Decision Rule

Use Terraform when the object should be centrally governed, low-frequency, and shared across teams/environments.
Use an application CI/CD pipeline when the object is product code, changes frequently, and is owned by a single app team.

### Practical Split (Recommended)

- Terraform owns platform guardrails and shared infrastructure.
- App repos own fast-moving prompt/function code and runtime content.

## Development

### Building

```sh
go build ./...
```

### Running Acceptance Tests

Acceptance tests run against a live Braintrust API. You'll need a valid API key:

```sh
export BRAINTRUST_API_KEY="sk-..."
export TF_ACC=1
go test -v ./internal/provider/ -timeout 600s
```

### Generating Documentation

```sh
go generate ./...
```

## License

This project is licensed under the [MPL-2.0](LICENSE).
