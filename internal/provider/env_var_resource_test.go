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

func TestAccEnvVarResource_basic(t *testing.T) {
	resourceName := "braintrust_env_var.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnvVarDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccEnvVarResourceConfig(rName, "MY_SECRET", "secret-value-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "MY_SECRET"),
					resource.TestCheckResourceAttr(resourceName, "object_type", "project"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "object_id"),
				),
			},
			// ImportState (value is write-only so it won't be verified).
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value"},
			},
			// Update the value.
			{
				Config: testAccEnvVarResourceConfig(rName, "MY_SECRET", "secret-value-2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "MY_SECRET"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			// Update the name (forces PATCH with new name).
			{
				Config: testAccEnvVarResourceConfig(rName, "MY_SECRET_RENAMED", "secret-value-2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "MY_SECRET_RENAMED"),
				),
			},
		},
	})
}

func testAccCheckEnvVarDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_env_var" {
			continue
		}
		_, err := c.GetEnvVar(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust env var %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("unexpected error checking env var %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccEnvVarResourceConfig(projectName, envVarName, envVarValue string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_env_var" "test" {
  object_type = "project"
  object_id   = braintrust_project.test.id
  name        = %[2]q
  value       = %[3]q
}
`, projectName, envVarName, envVarValue)
}
