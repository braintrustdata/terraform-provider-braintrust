# Agent-assisted Braintrust Terraform starter

Copy `AGENTS.md` and `main.tf` into a new Terraform directory in your own repo. Give your coding agent the desired Braintrust resources and organization ID, then ask it to edit `main.tf` and show you a `terraform plan`. The instructions in `AGENTS.md` travel with your configuration and work with agents that read that convention.

Set `BRAINTRUST_API_KEY` in your environment and provide `org_id` through a non-secret Terraform variable value. This example creates a project, puts it in a project group, and grants a permission group read access to the project group. Add users to the permission group with `data "braintrust_user"` and `member_users` when you are ready to grant actual users access.

The provider is not yet published in the Terraform Registry. To use this starter now, build the provider locally with `go build -o /absolute/path/to/bin/terraform-provider-braintrust .` from the provider repository, then create a Terraform CLI config file outside your project:

```hcl
provider_installation {
  dev_overrides {
    "braintrustdata/braintrust" = "/absolute/path/to/bin"
  }
  direct {}
}
```

Set `TF_CLI_CONFIG_FILE` to that file's absolute path. With the development override, run `terraform fmt -check`, `terraform validate`, and `terraform plan` without `terraform init`. After Registry publication, remove the override and use normal `terraform init`.

For resource details, see the [provider documentation](https://github.com/braintrustdata/terraform-provider-braintrust/tree/main/docs). For larger patterns, see the [complete examples](../complete/).
