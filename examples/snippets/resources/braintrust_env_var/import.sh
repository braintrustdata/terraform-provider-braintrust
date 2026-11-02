# Import by the env var ID. Omit both value and value_version to adopt without rotation.
# To update later, supply value through an ephemeral sensitive variable and set
# value_version to a new positive integer. Changing value alone does nothing.
# A new value_version with value omitted or null explicitly clears the remote value.
terraform import braintrust_env_var.example 00000000-0000-0000-0000-000000000000
