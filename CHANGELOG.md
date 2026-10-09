# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `braintrust_view` for saved views and monitor dashboards, with chart definitions in `view_data_json.custom_charts`.
- A scheduled API contract check against the public `braintrust-openapi` sync that maintains one review issue for drift.

## [0.1.0] - 2026-10-09

### Added

- Initial release of the Braintrust Terraform provider.
- **Resources**: `braintrust_project`, `braintrust_dataset`, `braintrust_group`,
  `braintrust_project_group`, `braintrust_project_group_membership`, `braintrust_prompt`,
  `braintrust_role`, `braintrust_acl`, `braintrust_env_var`, `braintrust_ai_secret`,
  `braintrust_service_account`, `braintrust_service_token`, `braintrust_project_score`,
  `braintrust_project_tag`, `braintrust_project_automation`.
- **Data sources**: `braintrust_project`, `braintrust_dataset`, `braintrust_group`,
  `braintrust_project_group`, `braintrust_prompt`, `braintrust_role`, `braintrust_user`.
- Name-based lookups for project and dataset data sources.
- CI/CD workflows for build checks, linting, and registry publishing.
- State security documentation covering sensitive values, locking, and operational caveats.
