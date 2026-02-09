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

func TestAccAPIKeyResource_basic(t *testing.T) {
	resourceName := "braintrust_api_key.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAPIKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccAPIKeyResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttrSet(resourceName, "key"),
					resource.TestCheckResourceAttrSet(resourceName, "preview_name"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
				),
			},
		},
	})
}

func testAccCheckAPIKeyDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_api_key" {
			continue
		}
		_, err := c.GetAPIKey(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust API key %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("unexpected error checking API key %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccAPIKeyResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_api_key" "test" {
  name = %[1]q
}
`, name)
}
