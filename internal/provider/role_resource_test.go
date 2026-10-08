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

func TestAccRoleResource_basic(t *testing.T) {
	resourceName := "braintrust_role.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	rNameUpdated := rName + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRoleDestroy,
		Steps: []resource.TestStep{
			// Step 1: Create.
			{
				Config: testAccRoleResourceConfig(rName, "Acceptance test role"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "description", "Acceptance test role"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "org_id"),
					resource.TestCheckResourceAttr(resourceName, "member_permissions.#", "2"),
				),
			},
			// Step 2: Import.
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Step 3: Update name and description.
			{
				Config: testAccRoleResourceConfig(rNameUpdated, "Updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rNameUpdated),
					resource.TestCheckResourceAttr(resourceName, "description", "Updated description"),
					resource.TestCheckResourceAttr(resourceName, "member_permissions.#", "2"),
				),
			},
		},
	})
}

func TestAccRoleResource_inheritedRoleWithoutDirectPermissions(t *testing.T) {
	name := "tf-acc-role-inherit-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	config := fmt.Sprintf(`
resource "braintrust_role" "read" {
  name = %q
  member_permissions = [{ permission = "read", restrict_object_type = "project" }]
}
resource "braintrust_role" "parent" {
  name = %q
  member_roles = [braintrust_role.read.id]
}
`, name+"-read", name+"-parent")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRoleDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("braintrust_role.parent", "member_permissions.#", "0"),
					resource.TestCheckResourceAttr("braintrust_role.parent", "member_roles.#", "1"),
				),
			},
			{
				ResourceName:      "braintrust_role.parent",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckRoleDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_role" {
			continue
		}
		_, err := c.GetRole(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust role %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("unexpected error checking role %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccRoleResourceConfig(name, description string) string {
	return fmt.Sprintf(`
resource "braintrust_role" "test" {
  name        = %[1]q
  description = %[2]q

  member_permissions = [
    {
      permission           = "create"
      restrict_object_type = "dataset"
    },
    {
      permission = "read"
    },
  ]
}
`, name, description)
}
