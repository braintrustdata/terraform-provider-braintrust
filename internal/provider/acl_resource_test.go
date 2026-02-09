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

func TestAccACLResource_withRole(t *testing.T) {
	resourceName := "braintrust_acl.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckACLDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccACLResourceWithRoleConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "object_type", "project"),
					resource.TestCheckResourceAttrSet(resourceName, "object_id"),
					resource.TestCheckResourceAttrSet(resourceName, "group_id"),
					resource.TestCheckResourceAttrSet(resourceName, "role_id"),
					resource.TestCheckResourceAttrSet(resourceName, "object_org_id"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccACLResource_withPermission(t *testing.T) {
	resourceName := "braintrust_acl.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckACLDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccACLResourceWithPermissionConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "object_type", "project"),
					resource.TestCheckResourceAttr(resourceName, "permission", "read"),
					resource.TestCheckResourceAttrSet(resourceName, "group_id"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckACLDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_acl" {
			continue
		}
		_, err := c.GetACL(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust ACL %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("unexpected error checking ACL %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccACLResourceWithRoleConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_group" "test" {
  name = %[1]q
}

data "braintrust_role" "viewer" {
  name = "Viewer"
}

resource "braintrust_acl" "test" {
  object_type = "project"
  object_id   = braintrust_project.test.id
  group_id    = braintrust_group.test.id
  role_id     = data.braintrust_role.viewer.id
}
`, name)
}

func testAccACLResourceWithPermissionConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_group" "test" {
  name = %[1]q
}

resource "braintrust_acl" "test" {
  object_type = "project"
  object_id   = braintrust_project.test.id
  group_id    = braintrust_group.test.id
  permission  = "read"
}
`, name)
}
