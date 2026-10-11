resource "braintrust_project" "example" {
  name = "functions-example"
}

resource "braintrust_function" "prompt" {
  project_id = braintrust_project.example.id
  name       = "Summarizer"
  slug       = "summarizer"

  function_data_json = jsonencode({ type = "prompt" })
  prompt_data_json = jsonencode({
    prompt  = { type = "completion", content = "Summarize {{input}}" }
    options = { model = "gpt-4o-mini" }
  })
}

resource "braintrust_function" "scorer" {
  project_id    = braintrust_project.example.id
  name          = "Pass scorer"
  slug          = "pass-scorer"
  function_type = "scorer"

  function_data_json = jsonencode({
    type = "code"
    data = {
      type            = "inline"
      runtime_context = { runtime = "quickjs", version = "ES2023" }
      code            = "function main() { return 1; }"
    }
  })
}

resource "braintrust_function" "preprocessor" {
  project_id    = braintrust_project.example.id
  name          = "Input text"
  slug          = "input-text"
  function_type = "preprocessor"

  function_data_json = jsonencode({
    type = "code"
    data = {
      type            = "inline"
      runtime_context = { runtime = "quickjs", version = "ES2023" }
      code            = "function main(spanData) { return { text: String(spanData.input || '') }; }"
    }
  })
}
