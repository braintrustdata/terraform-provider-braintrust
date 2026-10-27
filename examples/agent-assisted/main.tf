terraform {
  required_providers {
    braintrust = {
      source = "braintrustdata/braintrust"
      # version = "x.y.z" # pin a specific version
    }
  }
}

provider "braintrust" {}

variable "org_id" {
  description = "ID of the Braintrust organization selected for this Terraform state"
  type        = string
}

resource "braintrust_project" "app" {
  name = "example-app"
}

resource "braintrust_project_group" "apps" {
  org_id = var.org_id
  name   = "Example apps"
}

resource "braintrust_project_group_membership" "app" {
  project_group_id = braintrust_project_group.apps.id
  project_id       = braintrust_project.app.id
}

resource "braintrust_group" "readers" {
  name = "Example app readers"
}

resource "braintrust_acl" "readers" {
  object_type = "project_group"
  object_id   = braintrust_project_group.apps.id
  group_id    = braintrust_group.readers.id
  permission  = "read"
}
