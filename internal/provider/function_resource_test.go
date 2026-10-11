package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFunctionResource_types(t *testing.T) {
	name := "tf-acc-functions-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFunctionConfig(name, "function main() { return 0.5; }"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("braintrust_function.scorer", "id"),
					resource.TestCheckResourceAttr("braintrust_function.scorer", "function_type", "scorer"),
					resource.TestCheckResourceAttr("braintrust_function.preprocessor", "function_type", "preprocessor"),
					resource.TestCheckResourceAttrSet("braintrust_function.prompt", "id"),
					resource.TestCheckResourceAttrPair("data.braintrust_function.by_name", "id", "braintrust_function.scorer", "id"),
				),
			},
			{ResourceName: "braintrust_function.scorer", ImportState: true, ImportStateVerify: true},
			{Config: testAccFunctionConfig(name, "function main() { return 0.75; }"), Check: resource.TestCheckResourceAttr("braintrust_function.scorer", "description", "Terraform scorer")},
		},
	})
}

func testAccFunctionConfig(name, scorerBody string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}
resource "braintrust_function" "scorer" {
  project_id = braintrust_project.test.id
  name = "Terraform scorer"
  slug = "terraform-scorer"
  description = "Terraform scorer"
  function_type = "scorer"
  function_data_json = jsonencode({
    type = "code"
    data = {
      type = "inline"
      runtime_context = { runtime = "quickjs", version = "ES2023" }
      code = %[2]q
    }
  })
}
resource "braintrust_function" "preprocessor" {
  project_id = braintrust_project.test.id
  name = "Terraform preprocessor"
  slug = "terraform-preprocessor"
  function_type = "preprocessor"
  function_data_json = jsonencode({
    type = "code"
    data = {
      type = "inline"
      runtime_context = { runtime = "quickjs", version = "ES2023" }
      code = "function main(spanData) { return { text: String(spanData.input || '') }; }"
    }
  })
}
resource "braintrust_function" "prompt" {
  project_id = braintrust_project.test.id
  name = "Terraform prompt"
  slug = "terraform-prompt"
  function_data_json = jsonencode({ type = "prompt" })
  prompt_data_json = jsonencode({
    prompt = { type = "completion", content = "Summarize {{input}}" }
    options = { model = "gpt-4o-mini" }
  })
}
data "braintrust_function" "by_name" {
  project_id = braintrust_project.test.id
  name = braintrust_function.scorer.name
}
`, name, scorerBody)
}
