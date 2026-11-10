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

## Service accounts and ACL deletion

Reference a managed service account directly in an ACL:

```hcl
resource "braintrust_service_account" "ci" {
  name = "ci"
}

resource "braintrust_acl" "ci_read" {
  object_type = "project"
  object_id   = braintrust_project.example.id
  user_id     = braintrust_service_account.ci.id
  permission  = "read"
}
```

The reference makes Terraform delete the ACL before deleting or replacing the account and use the new account ID when recreating the ACL. A copied UUID does not establish that dependency.

### Recovering after confirmed external account deletion

Removing a service account through the organization-members API was observed to remove both its membership and its ACLs. Reads and deletes of the old ACL returned HTTP 400 with `Missing read access ... or the acl does not exist`. Terraform can recognize that ACL as missing, but cannot detect external service-account deletion: the public API has no service-account read endpoint, so refresh preserves the last known account ID. Applying without replacing the account can therefore attempt to recreate the ACL with a stale `user_id`. That invalid creation returned HTTP 500 in the reproduced failure; it did not demonstrate a surviving orphaned ACL.

Only after confirming that the account was deleted outside Terraform, explicitly replace its Terraform resource:

```shell
terraform apply -replace=braintrust_service_account.ci
```

Use the affected resource's full address in place of `braintrust_service_account.ci`, including any module path or instance key. Review the replacement plan before approving it. Keep ACL `user_id` values as direct resource references, as above, so Terraform recreates them with the replacement account's ID.

Recreating a service account with the same name creates a **new UUID**; the name does not restore its old identity or repair copied UUIDs. Do not infer account deletion from an API 500 or replace an account solely because an ACL operation failed.

### Persistent ACL read or delete errors

An ACL **read or delete** that persistently returns HTTP 500 is a different failure. The provider reports the error and retains the ACL state; a server error is not evidence that the ACL is gone. Contact Braintrust support with the ACL ID, account ID, and failing API response for investigation and any necessary server-side cleanup. Do not use `terraform state rm` as a cleanup mechanism: it only forgets the ACL locally.
