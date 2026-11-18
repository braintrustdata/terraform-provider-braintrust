# Kitchen Sink

Every resource and data source in the provider, built around one project. Use it to see how the pieces connect. The other examples here are better starting points for real configurations.

| Area | Resources |
|---|---|
| Project contents | `project`, `dataset`, `project_score`, `project_tag`, `function` (prompt, scorer, preprocessor, and Topics facet), `mcp_server`, `view` (saved log view and dashboard) |
| Secrets | `ai_secret` (org-wide AI provider), `env_var` (project AI provider and plain environment variable), including provider-specific metadata |
| Automations | `project_automation` (alert, retention, and Topics) |
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
- `braintrust_ai_secret` and `braintrust_env_var` overwrite existing entries with the same name and scope.
- AI secret and environment-variable values are write-only. Increment `secret_version` or `value_version` when changing the corresponding value; a value-only change does nothing. Counters remain in state and must not be ephemeral.
- The AI key variable is both ephemeral and sensitive, keeping its input out of plans and state. Existing state histories and backups may still contain previously stored values.
- To adopt an existing AI secret or environment variable without rotation, omit both its value and version. For an environment-variable update, a new nonnull version with a null or omitted value explicitly clears the remote value.
- Both AI provider examples configure `metadata`. Updates replace the entire object, including nested-key removal; omitting the attribute leaves remote metadata unmanaged. Set `metadata = jsonencode({})` to clear metadata without rotating the key. Org-level metadata replacement requires an API supporting `shallow_replace_json_fields`.
- Metadata can be loaded from JSON with `file("${path.module}/provider.json")`, or YAML with `jsonencode(yamldecode(file("${path.module}/provider.yaml")))`. The file contains the provider metadata object, not the API key. These files are optional alternatives to the inline examples; when using `make run-example`, its isolated workspace must also contain them.
- Metadata is marked sensitive but is stored in Terraform state. Keep API keys in the write-only `secret`/`value` attributes.

Clean up with `terraform destroy` and the same variables.
