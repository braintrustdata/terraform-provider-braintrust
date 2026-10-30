# List every user in the organization.
data "braintrust_users" "all" {}

# Look up several users at once. Emails without a Braintrust account are
# omitted from `users` instead of failing.
data "braintrust_users" "team" {
  emails = ["alice@example.com", "bob@example.com", "carol@example.com"]
}

locals {
  team_user_ids = { for u in data.braintrust_users.team.users : u.email => u.id }
}

resource "braintrust_group" "engineering" {
  name         = "Engineering"
  member_users = values(local.team_user_ids)
}
