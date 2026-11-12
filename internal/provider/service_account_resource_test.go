package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccServiceAccountResource(t *testing.T) {
	resourceName := "braintrust_service_account.test"
	name := fmt.Sprintf("tf-acc-service-account-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccServiceAccountConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestMatchResourceAttr(resourceName, "email", regexp.MustCompile(".+")),
				),
			},
		},
	})
}

func TestAccServiceAccountResource_externalDeletionRecovery(t *testing.T) {
	const accountResource = "braintrust_service_account.test"
	const aclResource = "braintrust_acl.test"
	name := "tf-acc-recovery-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	config := testAccServiceAccountWithACLConfig(name)
	var c *client.Client
	var originalAccountID, originalACLID string

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			var err error
			c, err = client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
			if err != nil {
				t.Fatal(err)
			}
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeTestCheckFunc(
			testAccCheckACLDestroy,
			testAccCheckProjectDestroy,
		),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(accountResource, "id"),
					resource.TestCheckResourceAttrSet(aclResource, "id"),
					resource.TestCheckResourceAttrPair(aclResource, "user_id", accountResource, "id"),
					func(s *terraform.State) error {
						originalAccountID = s.RootModule().Resources[accountResource].Primary.ID
						originalACLID = s.RootModule().Resources[aclResource].Primary.ID
						acl, err := getACL(context.Background(), c, originalACLID)
						if err != nil {
							return err
						}
						if nullableUUIDToTF(acl.UserId).ValueString() != originalAccountID {
							return fmt.Errorf("initial ACL is not attached to the service account")
						}
						return nil
					},
				),
			},
			{
				PreConfig: func() {
					_, err := patchOrganizationMembers(context.Background(), c, map[string]any{
						"remove_users": map[string]any{"ids": []string{originalAccountID}},
					})
					if err != nil {
						t.Fatalf("remove service account outside Terraform: %v", err)
					}
					_, err = getACL(context.Background(), c, originalACLID)
					if !client.IsNotFoundError(err) {
						t.Fatalf("expected the removed account's ACL to be missing, got: %v", err)
					}
				},
				// Explicit replacement is required: refresh cannot read service
				// accounts and must not infer deletion from an ACL error.
				Taint:  []string{accountResource},
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(accountResource, "name", name),
					resource.TestCheckResourceAttrSet(accountResource, "id"),
					resource.TestCheckResourceAttrSet(aclResource, "id"),
					resource.TestCheckResourceAttrPair(aclResource, "user_id", accountResource, "id"),
					func(s *terraform.State) error {
						accountID := s.RootModule().Resources[accountResource].Primary.ID
						aclID := s.RootModule().Resources[aclResource].Primary.ID
						if accountID == originalAccountID {
							return fmt.Errorf("same-name replacement reused deleted service-account ID %s", accountID)
						}
						if aclID == originalACLID {
							return fmt.Errorf("recovery reused deleted ACL ID %s", aclID)
						}
						acl, err := getACL(context.Background(), c, aclID)
						if err != nil {
							return err
						}
						if nullableUUIDToTF(acl.UserId).ValueString() != accountID {
							return fmt.Errorf("recreated ACL is not attached to replacement account %s", accountID)
						}
						return nil
					},
				),
			},
		},
	})
}

func testAccServiceAccountConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_service_account" "test" {
  name = %[1]q
}
`, name)
}

func testAccServiceAccountWithACLConfig(name string) string {
	return testAccServiceAccountConfig(name) + fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_acl" "test" {
  object_type = "project"
  object_id   = braintrust_project.test.id
  user_id     = braintrust_service_account.test.id
  permission  = "read"
}
`, name)
}
