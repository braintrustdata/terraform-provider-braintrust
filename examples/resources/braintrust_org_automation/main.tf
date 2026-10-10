# Organization-wide retention policies require a plan with custom retention.
# Keep one policy per object type in an organization.
resource "braintrust_org_automation" "experiment_retention" {
  org_id = var.org_id
  name   = "Experiment retention"

  retention_config = {
    object_type    = "experiment"
    retention_days = 90
  }
}

variable "org_id" {
  type = string
}
