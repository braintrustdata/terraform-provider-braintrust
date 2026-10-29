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

  # Code scorers define a function named handler. QuickJS is only supported
  # for preprocessors; use node or python for scorers.
  function_data_json = jsonencode({
    type = "code"
    data = {
      type            = "inline"
      runtime_context = { runtime = "node", version = "20" }
      code            = "function handler({ output }) { return output ? 1 : 0; }"
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
      code            = "function handler(spanData) { return { text: String(spanData.input || '') }; }"
    }
  })
}
