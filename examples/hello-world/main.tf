# The smallest useful Braintrust configuration: a project with a prompt to
# evaluate, a dataset to evaluate it against, and a scorer to grade the results.
#
#   export BRAINTRUST_API_KEY="..."
#   terraform init && terraform apply

terraform {
  required_providers {
    braintrust = {
      source = "braintrustdata/braintrust"
      # version = "x.y.z" # pin a specific version
    }
  }
}

provider "braintrust" {}

resource "braintrust_project" "app" {
  name        = "hello-world"
  description = "My first Terraform-managed Braintrust project"
}

resource "braintrust_function" "greeter" {
  project_id = braintrust_project.app.id
  name       = "Greeter"
  slug       = "greeter"

  function_data_json = jsonencode({ type = "prompt" })
  prompt_data_json = jsonencode({
    prompt = {
      type = "chat"
      messages = [
        { role = "system", content = "You greet people by name in one short sentence." },
        { role = "user", content = "{{input}}" },
      ]
    }
    options = { model = "gpt-4o-mini" }
  })
}

resource "braintrust_dataset" "golden" {
  project_id  = braintrust_project.app.id
  name        = "golden"
  description = "Curated test cases to evaluate against"
}

resource "braintrust_function" "mentions_name" {
  project_id    = braintrust_project.app.id
  name          = "Mentions name"
  slug          = "mentions-name"
  function_type = "scorer"

  function_data_json = jsonencode({
    type = "code"
    data = {
      type            = "inline"
      runtime_context = { runtime = "python", version = "3.12" }
      code            = <<-PYTHON
        def handler(input, output, expected, metadata):
            if expected is None:
                return None
            return 1.0 if expected.lower() in str(output).lower() else 0.0
      PYTHON
    }
  })
}

output "project_id" {
  value = braintrust_project.app.id
}
