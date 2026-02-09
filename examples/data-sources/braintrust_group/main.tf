# Look up the built-in Engineers group
data "braintrust_group" "engineers" {
  name = "Engineers"
}

output "engineers_group_id" {
  value = data.braintrust_group.engineers.id
}
