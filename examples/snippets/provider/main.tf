terraform {
  required_providers {
    braintrust = {
      source = "braintrustdata/braintrust"
      # version = "x.y.z" <--- pin to latest version
    }
  }
}

provider "braintrust" {
  # can be configured with envars:
  #  BRAINTRUST_API_URL
  #  BRAINTRUST_API_KEY
  # *or* use these settings (settings win over envars)
  api_url = "https://api.braintrust.dev" # <-- the default
  # api_key = "sk-..." <-- recommended to use envar instead
}

resource "braintrust_project" "hello" {
  name        = "hello-terraform"
  description = "Created by the Braintrust Terraform provider"
}

output "project_id" {
  value = braintrust_project.hello.id
}
