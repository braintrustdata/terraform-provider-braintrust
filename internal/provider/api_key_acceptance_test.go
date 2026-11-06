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

func TestAccAPIKeyResource_lifecycle(t *testing.T) {
	orgID := os.Getenv("BRAINTRUST_ORG_ID")
	if orgID == "" {
		t.Skip("BRAINTRUST_ORG_ID is required for API key acceptance tests; authenticate with an unrestricted user key")
	}
	name := "tf-acc-key-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	const resourceName = "braintrust_api_key.test"
	var originalID, originalKey string
	config := func(seconds int) string {
		return fmt.Sprintf(`
resource "braintrust_api_key" "test" {
  name = %q
  org_id = %q
  expires_in_seconds = %d
}
data "braintrust_api_key" "test" {
  id = braintrust_api_key.test.id
}
`, name, orgID, seconds)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAPIKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(3600),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "org_id", orgID),
					resource.TestCheckResourceAttrSet(resourceName, "key"),
					resource.TestCheckResourceAttrSet(resourceName, "expires_at"),
					resource.TestCheckResourceAttrPair("data.braintrust_api_key.test", "user_id", resourceName, "user_id"),
					func(s *terraform.State) error {
						attrs := s.RootModule().Resources[resourceName].Primary.Attributes
						originalID, originalKey = attrs["id"], attrs["key"]
						return nil
					},
				),
			},
			{
				Config: config(3600),
				Check: func(s *terraform.State) error {
					attrs := s.RootModule().Resources[resourceName].Primary.Attributes
					if attrs["id"] != originalID || attrs["key"] != originalKey {
						return fmt.Errorf("refresh changed the API key ID or lost its raw key")
					}
					return nil
				},
			},
			{
				ResourceName: resourceName, ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"key", "expires_in_seconds"},
			},
			{
				Config: config(7200),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "expires_in_seconds", "7200"),
					resource.TestCheckResourceAttrSet(resourceName, "key"),
					func(s *terraform.State) error {
						if s.RootModule().Resources[resourceName].Primary.ID == originalID {
							return fmt.Errorf("changing the lifetime did not replace the API key")
						}
						c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
						if err != nil {
							return err
						}
						_, err = getAPIKey(context.Background(), c, originalID)
						if !client.IsNotFoundError(err) {
							return fmt.Errorf("replacement did not confirm revocation of the old key")
						}
						return nil
					},
				),
			},
		},
	})
}

func testAccCheckAPIKeyDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_api_key" {
			continue
		}
		_, err := getAPIKey(context.Background(), c, rs.Primary.ID)
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("could not confirm API key %s was revoked", rs.Primary.ID)
		}
	}
	return nil
}
