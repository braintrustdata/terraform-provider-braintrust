# Terraform Provider for Braintrust

A [Terraform](https://www.terraform.io/) provider for managing [Braintrust](https://www.braintrustdata.com/) resources as code.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.23 (to build the provider plugin)

## Resources

| Resource | Description |
|---|---|
| `braintrust_project` | Projects for organizing experiments, datasets, and logs |
| `braintrust_dataset` | Datasets belonging to a project |
| `braintrust_group` | User groups for access control |
| `braintrust_role` | Custom roles with fine-grained permissions |
| `braintrust_acl` | Access control list entries (bind groups to roles/permissions on objects) |
| `braintrust_env_var` | Environment variables scoped to org, project, or function |
| `braintrust_ai_secret` | AI provider credentials (OpenAI, Anthropic, etc.) at org level |
| `braintrust_api_key` | API keys for user-level authentication |
| `braintrust_service_token` | Service tokens for programmatic access via service accounts |
| `braintrust_project_score` | Score definitions for labeling experiments |
| `braintrust_project_tag` | Tags for filtering experiments and logs |
| `braintrust_project_automation` | Automation rules (alerts, exports, retention, env notifications) |

## Data Sources

| Data Source | Description |
|---|---|
| `braintrust_project` | Look up a project by ID or name |
| `braintrust_dataset` | Look up a dataset by ID or by name + project |
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

See [LICENSE](LICENSE) for details.
