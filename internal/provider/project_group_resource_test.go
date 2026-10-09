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

func TestAccProjectGroupResource_membershipAndACL(t *testing.T) {
	orgID := os.Getenv("BRAINTRUST_ORG_ID")
	if orgID == "" {
		t.Skip("BRAINTRUST_ORG_ID is required for project group acceptance tests")
	}
	name := "tf-acc-pg-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectGroupDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectGroupConfig(orgID, name, "[braintrust_project.a.id, braintrust_project.b.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("braintrust_project_group.test", "member_projects.#", "2"),
					resource.TestCheckResourceAttrPair("data.braintrust_project_group.test", "id", "braintrust_project_group.test", "id"),
					resource.TestCheckResourceAttrPair("braintrust_acl.test", "object_id", "braintrust_project_group.test", "id"),
				),
			},
			{
				ResourceName:      "braintrust_project_group.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccProjectGroupConfig(orgID, name, "[braintrust_project.b.id]"),
				Check:  resource.TestCheckResourceAttr("braintrust_project_group.test", "member_projects.#", "1"),
			},
			{
				Config: testAccProjectGroupConfig(orgID, name, "[]"),
				Check:  resource.TestCheckResourceAttr("braintrust_project_group.test", "member_projects.#", "0"),
			},
		},
	})
}

func testAccProjectGroupConfig(orgID, name, members string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "a" { name = %[4]q }
resource "braintrust_project" "b" { name = %[5]q }
resource "braintrust_group" "readers" { name = %[6]q }
resource "braintrust_project_group" "test" {
  org_id = %[1]q
  name = %[2]q
  member_projects = %[3]s
}
data "braintrust_project_group" "test" {
  org_id = %[1]q
  name = braintrust_project_group.test.name
}
resource "braintrust_acl" "test" {
  object_type = "project_group"
  object_id = braintrust_project_group.test.id
  group_id = braintrust_group.readers.id
  permission = "read"
}
`, orgID, name, members, name+"-a", name+"-b", name+"-readers")
}

func testAccCheckProjectGroupDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_project_group" {
			continue
		}
		if _, err := c.GetProjectGroup(context.Background(), rs.Primary.ID); err == nil {
			return fmt.Errorf("project group %s still exists", rs.Primary.ID)
		} else if !client.IsNotFoundError(err) {
			return err
		}
	}
	return nil
}

func TestAccProjectGroupMembership_dependencyOrder(t *testing.T) {
	orgID := os.Getenv("BRAINTRUST_ORG_ID")
	if orgID == "" {
		t.Skip("BRAINTRUST_ORG_ID is required for project group acceptance tests")
	}
	name := "tf-acc-pgm-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectGroupDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMembershipConfig(orgID, name, true, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("braintrust_project_group_membership.alpha", "id"),
					resource.TestCheckResourceAttrSet("braintrust_project_group_membership.beta", "id"),
				),
			},
			{
				ResourceName:      "braintrust_project_group_membership.alpha",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{Config: testAccMembershipConfig(orgID, name, true, false)},
			// Removing the last membership and its project in the same apply
			// requires Terraform to remove the membership first.
			{Config: testAccMembershipConfig(orgID, name, false, false)},
		},
	})
}

func TestAccProjectGroupMembership_fullDestroy(t *testing.T) {
	orgID := os.Getenv("BRAINTRUST_ORG_ID")
	if orgID == "" {
		t.Skip("BRAINTRUST_ORG_ID is required for project group acceptance tests")
	}
	name := "tf-acc-pgm-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectGroupDestroy,
		Steps:                    []resource.TestStep{{Config: testAccMembershipConfig(orgID, name, true, true)}},
	})
}

func testAccMembershipConfig(orgID, name string, includeProject, includeAlpha bool) string {
	config := fmt.Sprintf(`
resource "braintrust_project_group" "alpha" {
  org_id = %[1]q
  name = %[2]q
}
resource "braintrust_project_group" "beta" {
  org_id = %[1]q
  name = %[3]q
}
`, orgID, name+"-alpha", name+"-beta")
	if includeProject {
		config += fmt.Sprintf(`
resource "braintrust_project" "member" { name = %q }
resource "braintrust_project_group_membership" "beta" {
  project_group_id = braintrust_project_group.beta.id
  project_id = braintrust_project.member.id
}
`, name+"-project")
		if includeAlpha {
			config += `
resource "braintrust_project_group_membership" "alpha" {
  project_group_id = braintrust_project_group.alpha.id
  project_id = braintrust_project.member.id
}
`
		}
	}
	return config
}
