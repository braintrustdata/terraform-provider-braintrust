# Look up a user by email.
data "braintrust_user" "alice" {
  email = "alice@example.com"
}

# Alternatively, look up a user by ID.
data "braintrust_user" "bob" {
  id = "00000000-0000-0000-0000-000000000000"
}

# Use the user data source to manage group membership by email.
resource "braintrust_group" "engineering" {
  name = "Engineering"
  member_users = [
    data.braintrust_user.alice.id,
    data.braintrust_user.bob.id,
  ]
}
