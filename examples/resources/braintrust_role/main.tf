# Create a custom role with specific permissions
resource "braintrust_role" "dataset_manager" {
  name        = "Dataset Manager"
  description = "Can read and update datasets, but not delete them"

  member_permissions {
    permission           = "read"
    restrict_object_type = "dataset"
  }

  member_permissions {
    permission           = "update"
    restrict_object_type = "dataset"
  }

  member_permissions {
    permission           = "create"
    restrict_object_type = "dataset"
  }
}

# Create a role that inherits from the built-in Viewer role
resource "braintrust_role" "viewer_plus" {
  name        = "Viewer Plus"
  description = "Viewer role plus the ability to create experiments"

  member_roles = [
    data.braintrust_role.viewer.id,
  ]

  member_permissions {
    permission           = "create"
    restrict_object_type = "experiment"
  }
}
