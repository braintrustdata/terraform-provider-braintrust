package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccProjectResource_basic(t *testing.T) {
	resourceName := "braintrust_project.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	rNameUpdated := rName + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			// Step 1: Create and Read
			{
				Config: testAccProjectResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "org_id"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
					resource.TestCheckResourceAttrSet(resourceName, "user_id"),
				),
			},
			// Step 2: ImportState - verify import works
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Step 3: Update name and Read
			{
				Config: testAccProjectResourceConfig(rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rNameUpdated),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
		},
	})
}

func TestAccProjectResource_withDescription(t *testing.T) {
	resourceName := "braintrust_project.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			// Step 1: Create with description
			{
				Config: testAccProjectResourceConfigWithDescription(
					rName,
					"A project for acceptance testing",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "description", "A project for acceptance testing"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			// Step 2: Update the description
			{
				Config: testAccProjectResourceConfigWithDescription(
					rName,
					"Updated description for acceptance testing",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "description", "Updated description for acceptance testing"),
				),
			},
			// Step 3: Remove the description (tests clearing optional fields)
			{
				Config: testAccProjectResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
				),
			},
		},
	})
}

func TestAccProjectResource_preservesUnmanagedSettings(t *testing.T) {
	name := "tf-acc-settings-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	resourceName := "braintrust_project.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf("resource \"braintrust_project\" \"test\" {\n  name = %q\n  comparison_key = \"input\"\n}\n", name),
				Check:  seedProjectBlindReviews(),
			},
			{
				Config: fmt.Sprintf("resource \"braintrust_project\" \"test\" {\n  name = %q\n  description = \"description update\"\n  comparison_key = \"input\"\n}\n", name),
				Check:  checkProjectBlindReviews(),
			},
			{
				Config: fmt.Sprintf("resource \"braintrust_project\" \"test\" {\n  name = %q\n  description = \"description update\"\n  comparison_key = \"output\"\n}\n", name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "comparison_key", "output"),
					checkProjectBlindReviews(),
				),
			},
		},
	})
}

func projectFromTestState(s *terraform.State) (*client.Client, string, error) {
	rs := s.RootModule().Resources["braintrust_project.test"]
	if rs == nil || rs.Primary == nil {
		return nil, "", fmt.Errorf("project missing from Terraform state")
	}
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	return c, rs.Primary.ID, err
}

func seedProjectBlindReviews() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c, id, err := projectFromTestState(s)
		if err != nil {
			return err
		}
		project, err := c.GetProject(context.Background(), id)
		if err != nil {
			return err
		}
		settings := map[string]json.RawMessage{}
		if project.Settings != nil {
			for k, v := range project.Settings.Raw {
				settings[k] = v
			}
		}
		settings["blind_reviews"] = json.RawMessage("true")
		_, err = c.UpdateProject(context.Background(), id, client.UpdateProjectRequest{Settings: settings})
		return err
	}
}

func checkProjectBlindReviews() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c, id, err := projectFromTestState(s)
		if err != nil {
			return err
		}
		project, err := c.GetProject(context.Background(), id)
		if err != nil {
			return err
		}
		if project.Settings == nil || string(project.Settings.Raw["blind_reviews"]) != "true" {
			return fmt.Errorf("project %s lost unmanaged blind_reviews setting", id)
		}
		return nil
	}
}

// testAccCheckProjectDestroy verifies the project has been destroyed.
func testAccCheckProjectDestroy(s *terraform.State) error {
	apiKey := os.Getenv(client.EnvAPIKey)
	apiURL := os.Getenv(client.EnvAPIURL)

	c, err := client.NewClient(apiURL, apiKey, "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_project" {
			continue
		}

		_, err := c.GetProject(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust project %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("unexpected error checking project %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

// --- Config helper functions ---

func testAccProjectResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}
`, name)
}

func testAccProjectResourceConfigWithDescription(name, description string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name        = %[1]q
  description = %[2]q
}
`, name, description)
}
