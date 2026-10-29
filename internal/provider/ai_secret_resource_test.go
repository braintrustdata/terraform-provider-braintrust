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

func TestAccAISecretResource_basic(t *testing.T) {
	resourceName := "braintrust_ai_secret.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	rNameUpdated := rName + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAISecretDestroy,
		Steps: []resource.TestStep{
			// Step 1: Create with name, type, and secret.
			{
				Config: testAccAISecretResourceConfig(rName, "openai", "sk-test-secret-value-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "type", "openai"),
					resource.TestCheckResourceAttrSet(resourceName, "org_id"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
				),
			},
			// Step 2: Import (secret is write-only, so ignore it).
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secret"},
			},
			// Step 3: Update name and rotate the secret.
			{
				Config: testAccAISecretResourceConfig(rNameUpdated, "openai", "sk-test-secret-value-2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rNameUpdated),
					resource.TestCheckResourceAttr(resourceName, "type", "openai"),
					resource.TestCheckResourceAttrSet(resourceName, "updated_at"),
				),
			},
		},
	})
}

func testAccCheckAISecretDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_ai_secret" {
			continue
		}
		_, err := c.API.GetAiSecretIdWithResponse(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust AI secret %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("unexpected error checking AI secret %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccAISecretResourceConfig(name, secretType, secret string) string {
	return fmt.Sprintf(`
resource "braintrust_ai_secret" "test" {
  name   = %[1]q
  type   = %[2]q
  secret = %[3]q
}
`, name, secretType, secret)
}

func TestAccAISecretResource_metadata(t *testing.T) {
	name := "tf-acc-metadata-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	config := func(deployment string) string {
		return secretMetadataConfig("ai_secret", name, "", fmt.Sprintf(`metadata = jsonencode({ api_base = "https://example.openai.azure.com", api_version = "2024-10-21", deployment = %q })`, deployment))
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAISecretDestroy,
		Steps: []resource.TestStep{
			{Config: config("first"), Check: resource.TestCheckResourceAttr("braintrust_ai_secret.test", "metadata", `{"api_base":"https://example.openai.azure.com","api_version":"2024-10-21","deployment":"first"}`)},
			{ResourceName: "braintrust_ai_secret.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"secret"}},
			{Config: config("second"), Check: resource.TestCheckResourceAttr("braintrust_ai_secret.test", "metadata", `{"api_base":"https://example.openai.azure.com","api_version":"2024-10-21","deployment":"second"}`)},
			{Config: config("second"), PlanOnly: true},
			{Config: secretMetadataConfig("ai_secret", name, "", ""), ExpectError: regexp.MustCompile("Cannot Remove AI Secret Metadata")},
			{Config: config("second"), PlanOnly: true},
		},
	})
}
