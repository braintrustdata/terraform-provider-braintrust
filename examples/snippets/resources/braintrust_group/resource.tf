# Look up users by email so we do not hardcode UUIDs.
data "braintrust_user" "alice" {
  email = "alice@example.com"
}

data "braintrust_user" "bob" {
  email = "bob@example.com"
}

# Create a group for the ML team.
resource "braintrust_group" "ml_team" {
  name        = "ML Team"
  description = "Machine learning engineers"
  member_users = [
    data.braintrust_user.alice.id,
    data.braintrust_user.bob.id,
  ]
}

# Create a group that inherits from another group
resource "braintrust_group" "ml_leads" {
  name        = "ML Leads"
  description = "ML team leads - inherits all ML Team members"
  member_groups = [
    braintrust_group.ml_team.id,
  ]
}
