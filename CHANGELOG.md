# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial version of the Braintrust Terraform provider.
- **Resources**: `braintrust_project`, `braintrust_dataset`, `braintrust_group`,
  `braintrust_project_group`, `braintrust_project_group_membership`, `braintrust_prompt`,
  `braintrust_role`, `braintrust_acl`, `braintrust_env_var`, `braintrust_ai_secret`,
  `braintrust_service_account`, `braintrust_service_token`, `braintrust_project_score`,
  `braintrust_project_tag`, `braintrust_project_automation`, `braintrust_org_automation`,
  `braintrust_view`, `braintrust_mcp_server`.
- **Data sources**: `braintrust_project`, `braintrust_dataset`, `braintrust_group`,
  `braintrust_project_group`, `braintrust_prompt`, `braintrust_role`, `braintrust_user`.
- Name-based lookups for project and dataset data sources.
- SQL-backed charts in `braintrust_view` dashboards.
- CI/CD workflows for build checks, linting, API drift review, sandbox acceptance tests,
  and registry publishing.
- State security documentation covering sensitive values, locking, and operational caveats.

### Fixed

- Reject successful create responses that identify an existing remote object, so Terraform
  cannot accidentally adopt and later delete a resource it did not create.
- Paginate list responses and reject ambiguous data source lookups.
