# Look up several users at once. Fails if any email has no Braintrust account.
data "braintrust_users" "myteam" {
  emails = ["alice@example.com", "bob@example.com", "carol@example.com"]
}

resource "braintrust_group" "engineering" {
  name         = "Engineering"
  member_users = data.braintrust_users.myteam.users[*].id
}

# Skip emails without an account instead of failing; they are listed in
# `missing_emails`.
data "braintrust_users" "invited" {
  emails         = ["dave@example.com", "erin@example.com"]
  ignore_missing = true
}

output "not_signed_up_yet" {
  value = data.braintrust_users.invited.missing_emails
}

# List every user in the organization.
data "braintrust_users" "all" {
  return_all = true
}
