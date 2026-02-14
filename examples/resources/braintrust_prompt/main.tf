resource "braintrust_project" "example" {
  name = "Prompt Example Project"
}

resource "braintrust_prompt" "welcome" {
  project_id    = braintrust_project.example.id
  name          = "Welcome Prompt"
  slug          = "welcome-prompt"
  description   = "Prompt used for welcome message generation."
  tags          = ["production", "chat"]
  function_type = "prompt"

  prompt_data_json = jsonencode({
    prompt = {
      type = "completion"
      content = "Write a short, friendly welcome message for {{user_name}}."
    }
    options = {
      model = "gpt-4o-mini"
    }
  })
}
