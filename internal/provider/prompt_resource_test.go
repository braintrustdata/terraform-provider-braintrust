package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPromptResource_basic(t *testing.T) {
	name := "tf-acc-prompt-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	resourceName := "braintrust_prompt.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPromptConfig(name, "first", `["audit"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "tags.#", "1"),
					resource.TestCheckResourceAttrPair("data.braintrust_prompt.test", "id", resourceName, "id"),
					resource.TestCheckResourceAttrPair("data.braintrust_prompt.test", "prompt_data_json", resourceName, "prompt_data_json"),
					resource.TestCheckResourceAttrPair("data.braintrust_prompt.by_name", "id", resourceName, "id"),
				),
			},
			{ResourceName: resourceName, ImportState: true, ImportStateVerify: true},
			{
				Config: testAccPromptConfig(name, "second", `[]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "tags.#", "0"),
					resource.TestCheckResourceAttrPair("data.braintrust_prompt.test", "xact_id", resourceName, "xact_id"),
				),
			},
		},
	})
}

func testAccPromptConfig(name, content, tags string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}
resource "braintrust_prompt" "test" {
  project_id = braintrust_project.test.id
  name = %[1]q
  slug = %[1]q
  function_type = "prompt"
  tags = %[3]s
  prompt_data_json = jsonencode({
    prompt = { type = "completion", content = %[2]q }
    options = { model = "gpt-4o-mini" }
  })
}
data "braintrust_prompt" "test" {
  id = braintrust_prompt.test.id
}
data "braintrust_prompt" "by_name" {
  project_id = braintrust_project.test.id
  name = braintrust_prompt.test.name
}
`, name, content, tags)
}
