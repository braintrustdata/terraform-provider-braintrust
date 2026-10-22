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

func TestAccServiceTokenResource_basic(t *testing.T) {
	resourceName := "braintrust_service_token.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	serviceAccountID := os.Getenv("BRAINTRUST_SERVICE_ACCOUNT_ID")
	if serviceAccountID == "" {
		t.Skip("BRAINTRUST_SERVICE_ACCOUNT_ID must be set for service token acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckServiceTokenDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccServiceTokenResourceConfig(rName, serviceAccountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttrSet(resourceName, "key"),
					resource.TestCheckResourceAttrSet(resourceName, "preview_name"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
					resource.TestCheckResourceAttr(resourceName, "service_account_id", serviceAccountID),
				),
			},
		},
	})
}

func TestAccServiceTokenResource_withManagedAccount(t *testing.T) {
	name := "tf-acc-token-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckServiceTokenDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccServiceTokenWithAccountConfig(name, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("braintrust_service_token.test", "id"),
					resource.TestCheckResourceAttrSet("braintrust_service_token.test", "key"),
					resource.TestCheckResourceAttr("braintrust_service_token.test", "expires_in_seconds", "3600"),
					resource.TestCheckResourceAttrSet("braintrust_service_token.test", "expires_at"),
					resource.TestCheckResourceAttrPair("braintrust_service_token.test", "service_account_id", "braintrust_service_account.test", "id"),
				),
			},
			{
				Config: testAccServiceTokenWithAccountConfig(name, name+"-replacement"),
				Check:  resource.TestCheckResourceAttr("braintrust_service_token.test", "name", name+"-replacement"),
			},
		},
	})
}

func testAccServiceTokenWithAccountConfig(accountName, tokenName string) string {
	return fmt.Sprintf(`
resource "braintrust_service_account" "test" {
  name = %[1]q
}
resource "braintrust_service_token" "test" {
  name               = %[2]q
  service_account_id = braintrust_service_account.test.id
  expires_in_seconds = 3600
}
`, accountName, tokenName)
}

func testAccCheckServiceTokenDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_service_token" {
			continue
		}
		_, err := c.API.GetServiceTokenIdWithResponse(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust service token %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("unexpected error checking service token %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccServiceTokenResourceConfig(name, serviceAccountID string) string {
	return fmt.Sprintf(`
resource "braintrust_service_token" "test" {
  name               = %[1]q
  service_account_id = %[2]q
}
`, name, serviceAccountID)
}
