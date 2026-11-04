# Import by the AI secret ID. Omit both secret and secret_version to adopt without rotation.
# To rotate later, supply secret through an ephemeral sensitive variable and set
# secret_version to a new positive integer. Changing secret alone does nothing.
terraform import braintrust_ai_secret.example 00000000-0000-0000-0000-000000000000
