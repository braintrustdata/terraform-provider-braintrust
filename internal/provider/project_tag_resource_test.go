package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccProjectTagResource_basic(t *testing.T) {
	resourceName := "braintrust_project_tag.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	rNameUpdated := rName + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectTagDestroy,
		Steps: []resource.TestStep{
			// Step 1: Create with name and color.
			{
				Config: testAccProjectTagResourceConfig(rName, "#ff0000"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "color", "#ff0000"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "project_id"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
				),
			},
			// Step 2: ImportState.
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Step 3: Update name, description, and color.
			{
				Config: testAccProjectTagResourceConfigUpdated(rNameUpdated, "a useful tag", "#00ff00"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rNameUpdated),
					resource.TestCheckResourceAttr(resourceName, "description", "a useful tag"),
					resource.TestCheckResourceAttr(resourceName, "color", "#00ff00"),
				),
			},
		},
	})
}

func testAccCheckProjectTagDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_project_tag" {
			continue
		}
		_, err := c.API.GetProjectTagIdWithResponse(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust project tag %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("unexpected error checking project tag %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccProjectTagResourceConfig(name, color string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_project_tag" "test" {
  project_id = braintrust_project.test.id
  name       = %[1]q
  color      = %[2]q
}
`, name, color)
}

func testAccProjectTagResourceConfigUpdated(name, description, color string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_project_tag" "test" {
  project_id  = braintrust_project.test.id
  name        = %[1]q
  description = %[2]q
  color       = %[3]q
}
`, name, description, color)
}
