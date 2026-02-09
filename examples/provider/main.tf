terraform {
  required_providers {
    braintrust = {
      source = "braintrustdata/braintrust"
    }
  }
}

# Configure the Braintrust provider.
# The API key can also be set via the BRAINTRUST_API_KEY environment variable.
provider "braintrust" {
  # api_key = "your-api-key-here"
  # api_url = "https://api.braintrust.dev"  # optional, defaults to this
}
