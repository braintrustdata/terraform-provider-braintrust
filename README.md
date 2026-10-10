# Terraform Provider for Braintrust

A [Terraform](https://www.terraform.io/) provider for managing [Braintrust](https://www.braintrustdata.com/) resources as code.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://go.dev/doc/install) >= 1.26.9 (to build the provider plugin)

## Resources

| Resource | Description |
|---|---|
| `braintrust_project` | Projects for organizing experiments, datasets, and logs |
| `braintrust_dataset` | Datasets belonging to a project |
| `braintrust_prompt` | Prompt definitions belonging to a project |
| `braintrust_group` | User groups for access control |
| `braintrust_project_group` | Project groups for inherited access control |
| `braintrust_project_group_membership` | Individual project-to-group membership with safe destroy ordering |
| `braintrust_role` | Custom roles with fine-grained permissions |
| `braintrust_acl` | Access control list entries (bind groups to roles/permissions on objects) |
| `braintrust_env_var` | Environment variables scoped to org, project, or function |
| `braintrust_ai_secret` | AI provider credentials (OpenAI, Anthropic, etc.) at org level |
| `braintrust_service_account` | Service accounts for non-human automation identities |
| `braintrust_service_token` | Service tokens for programmatic access via service accounts |
| `braintrust_project_score` | Score definitions for labeling experiments |
| `braintrust_project_tag` | Tags for filtering experiments and logs |
| `braintrust_project_automation` | Automation rules (alerts, exports, retention, env notifications) |
| `braintrust_org_automation` | Organization-wide retention policies for logs, experiments, or datasets (plan permitting) |
| `braintrust_view` | Saved views and monitor dashboards with SQL-backed charts |
| `braintrust_mcp_server` | Project-scoped MCP server connections |

## Data Sources

| Data Source | Description |
|---|---|
| `braintrust_project` | Look up a project by ID or name |
| `braintrust_dataset` | Look up a dataset by ID or by name + project |
| `braintrust_prompt` | Look up a prompt by ID or by name + project |
| `braintrust_group` | Look up a group by name |
| `braintrust_project_group` | Look up a project group by organization ID and name |
| `braintrust_role` | Look up a role by name (e.g. built-in `Viewer`, `Admin`) |
| `braintrust_user` | Look up a user by email or ID (useful for group membership) |

## Installation

Add the provider to your `required_providers` block:

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

The provider is not yet published in the Terraform Registry. For now, build it locally and use a [Terraform development override](examples/agent-assisted/README.md) to validate and plan. After publication, `terraform init` will install it normally.

Maintainers: see [RELEASING.md](RELEASING.md) for the Registry publication steps.

## Authentication

The provider accepts a Braintrust user API key or service token. You can create a user API key in the Braintrust [organization settings](https://www.braintrustdata.com/app/settings?subroute=api-keys).

The public Braintrust API does not currently permit creating user API keys, so the provider does not manage them. Create the key in organization settings before using Terraform. Service tokens can be managed with `braintrust_service_token` when authenticated with an organization-owner service token.

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

### Working With a Coding Agent

The [agent-assisted starter](examples/agent-assisted/) includes a copyable `AGENTS.md` for your own Terraform repo and a small project-group configuration. It points an agent to the installed provider schema, shows safe dependency references, and asks for a reviewed plan before applying changes.

### Saved Views and Dashboards

Use `braintrust_view` for saved views and monitor dashboards. A dashboard has
`view_type = "monitor"`; its chart layout and definitions live in
`view_data_json.custom_charts`. The [view example](examples/resources/braintrust_view/main.tf)
includes a SQL big-number chart and a grouped time series; its [SQL chart guide](examples/resources/braintrust_view/README.md)
shows the estimated-cost query from the chart editor. See the official
[Braintrust SQL charts documentation](https://www.braintrust.dev/docs/observe/dashboards/sql-charts#sql-charts)
for chart query behavior and options.
Charts share the view's lifecycle because the Braintrust API has no separate chart
resource. Import a view with `object_type/object_id/view_id`, for example
`project/PROJECT_UUID/VIEW_UUID`.

The [API drift workflow](.github/workflows/api-drift.yml) checks the public
[Braintrust OpenAPI sync](https://github.com/braintrustdata/braintrust-openapi)
on weekdays and maintains one review issue for API changes. It also accepts a
`braintrust-openapi-updated` repository dispatch from the internal sync pipeline.
After reviewing an issue and updating the provider and exporter as needed, refresh
`api-contract-baseline.json` with
`python3 scripts/check_api_drift.py --spec spec.json --refresh-baseline`.

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
export BRAINTRUST_TEST_ORG_ID="YOUR_DEDICATED_TEST_ORG_ID"
export BRAINTRUST_ORG_ID="$BRAINTRUST_TEST_ORG_ID"
export TF_ACC=1
go test -v ./internal/provider/ -timeout 1200s
```

For GitHub Actions, configure the repository secret `BRAINTRUST_ACCEPTANCE_API_KEY`
with a credential that can access **only** the dedicated test organization, and
set the repository variable `BRAINTRUST_TEST_ORG_ID`. Apply the
`run-acc-tests` label to a trusted same-repository pull request to run the
live suite. CI verifies the credential's organization scope before creating
resources and serializes sandbox runs. The optional `BRAINTRUST_TEST_USER_ID`
and `BRAINTRUST_TEST_USER_EMAIL` secrets enable user lookup and membership
tests. The test requiring a pre-existing service account skips unless
`BRAINTRUST_SERVICE_ACCOUNT_ID` is configured; a separate test creates and
destroys its own service account and token.

### Generating Documentation

```sh
go generate ./...
```

## License

This project is licensed under the [MPL-2.0](LICENSE).
