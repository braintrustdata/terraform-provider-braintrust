# Kitchen Sink

Every resource and data source in the provider, built around one project. Use it to see how the pieces connect. The other examples here are better starting points for real configurations.

| Area | Resources |
|---|---|
| Project contents | `project`, `dataset`, `project_score`, `project_tag`, `function` (prompt, scorer, preprocessor, and Topics facet), `mcp_server`, `view` (saved log view and dashboard) |
| Secrets | `ai_secret` (org-wide), `env_var` (project) |
| Automations | `project_automation` (alert, retention, and Topics), `org_automation` (optional) |
| Access control | `role`, `group`, `group_membership`, `acl`, `project_group`, `project_group_membership`, `service_account`, `service_token` |
| Data sources | `user`, `role`, `project`, `dataset`, `function`, `project_group`, `group` |

## Usage

Requires Terraform >= 1.11.

```bash
export BRAINTRUST_API_KEY="..."
terraform apply \
  -var="my_email=you@company.com" \
  -var="openai_api_key=..." \
  -var="owner_service_token=..."
```

- `owner_service_token` must be an organization-owner service token. It is only used to create `braintrust_service_token`.
- `braintrust_org_automation` requires a plan with custom retention. Set `org_experiment_retention_days` to create it.
- `braintrust_ai_secret` and `braintrust_env_var` overwrite existing entries with the same name and scope.
- AI secret and environment-variable values are write-only. Increment `secret_version` or `value_version` when changing the corresponding value; a value-only change does nothing. Counters remain in state and must not be ephemeral.
- The AI key variable is both ephemeral and sensitive, keeping its input out of plans and state. Existing state histories and backups may still contain previously stored values.
- To adopt an existing AI secret or environment variable without rotation, omit both its value and version. For an environment-variable update, a new nonnull version with a null or omitted value explicitly clears the remote value.

Clean up with `terraform destroy` and the same variables.
