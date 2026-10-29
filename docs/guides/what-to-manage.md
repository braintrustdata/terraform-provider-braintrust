---
page_title: "What to Manage in Terraform"
subcategory: ""
description: |-
  Which Braintrust objects fit Terraform, and which belong in application pipelines.
---

# What to Manage in Terraform

Terraform works best for stable, shared configuration, not high-churn application content.

Good fits for Terraform:

- Access control: groups, roles, ACLs, service accounts, service tokens.
- Scaffolding: projects, score definitions, tags, datasets (the containers, not their rows).
- Org configuration: AI secrets, environment variables, automations.

Better managed in application code and CI/CD:

- Frequently changing prompts and functions.
- Data and runtime events: dataset rows, logs, experiments.
- Anything that should ship with application releases.

Rule of thumb: use Terraform when an object is centrally governed, changes rarely, and is shared across teams or environments. Use the application's pipeline when it's product code owned by one team that changes often.
