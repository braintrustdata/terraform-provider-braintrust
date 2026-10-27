# Look up the built-in system roles
data "braintrust_role" "viewer" {
  name = "Viewer"
}

data "braintrust_role" "engineer" {
  name = "Engineer"
}

data "braintrust_role" "owner" {
  name = "Owner"
}

output "viewer_role_id" {
  value = data.braintrust_role.viewer.id
}
