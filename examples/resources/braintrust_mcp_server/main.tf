resource "braintrust_project" "tools" {
  name = "agent-tools"
}

resource "braintrust_mcp_server" "tools" {
  project_id  = braintrust_project.tools.id
  name        = "Shared tools"
  url         = "https://tools.example.com/mcp"
  description = "Project tools exposed through MCP"
}
