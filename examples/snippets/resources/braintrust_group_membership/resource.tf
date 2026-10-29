variable "org_id" {
  type = string
}

data "braintrust_group" "engineers" {
  name   = "Engineers"
  org_id = var.org_id
}

data "braintrust_user" "alice" {
  email = "alice@example.com"
}

resource "braintrust_group_membership" "alice_engineer" {
  group_id = data.braintrust_group.engineers.id
  user_id  = data.braintrust_user.alice.id
}
