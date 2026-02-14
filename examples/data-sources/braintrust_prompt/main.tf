data "braintrust_project" "example" {
  name = "Prompt Example Project"
}

data "braintrust_prompt" "welcome" {
  project_id = data.braintrust_project.example.id
  name       = "Welcome Prompt"
}
