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

func TestAccProjectScoreResource_basic(t *testing.T) {
	resourceName := "braintrust_project_score.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	rNameUpdated := rName + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectScoreDestroy,
		Steps: []resource.TestStep{
			// Step 1: Create a slider score.
			{
				Config: testAccProjectScoreResourceConfig(rName, "slider"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "score_type", "slider"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "project_id"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
				),
			},
			// Step 2: ImportState.
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Step 3: Update name, add description.
			{
				Config: testAccProjectScoreResourceConfigUpdated(rNameUpdated, "slider", "accuracy metric"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rNameUpdated),
					resource.TestCheckResourceAttr(resourceName, "score_type", "slider"),
					resource.TestCheckResourceAttr(resourceName, "description", "accuracy metric"),
				),
			},
		},
	})
}

func TestAccProjectScoreResource_scoreTypeReplaces(t *testing.T) {
	name := "tf-acc-score-type-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	var firstID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectScoreDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectScoreResourceConfig(name, "slider"),
				Check: func(s *terraform.State) error {
					firstID = s.RootModule().Resources["braintrust_project_score.test"].Primary.ID
					if firstID == "" {
						return fmt.Errorf("slider score has no ID")
					}
					return nil
				},
			},
			{
				Config: testAccProjectScoreResourceConfig(name, "categorical"),
				Check: func(s *terraform.State) error {
					score := s.RootModule().Resources["braintrust_project_score.test"]
					if score.Primary.ID == firstID {
						return fmt.Errorf("score type change kept ID %s; expected replacement", firstID)
					}
					if score.Primary.Attributes["score_type"] != "categorical" {
						return fmt.Errorf("unexpected replacement score type: %s", score.Primary.Attributes["score_type"])
					}
					return nil
				},
			},
		},
	})
}

func testAccCheckProjectScoreDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_project_score" {
			continue
		}
		_, err := c.GetProjectScore(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust project score %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("unexpected error checking project score %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccProjectScoreResourceConfig(name, scoreType string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_project_score" "test" {
  project_id = braintrust_project.test.id
  name       = %[1]q
  score_type = %[2]q
}
`, name, scoreType)
}

func testAccProjectScoreResourceConfigUpdated(name, scoreType, description string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_project_score" "test" {
  project_id  = braintrust_project.test.id
  name        = %[1]q
  score_type  = %[2]q
  description = %[3]q
}
`, name, scoreType, description)
}
