# Create a group for the ML team
resource "braintrust_group" "ml_team" {
  name        = "ML Team"
  description = "Machine learning engineers"
  member_users = [
    "user-uuid-1",
    "user-uuid-2",
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
