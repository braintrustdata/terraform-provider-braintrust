package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

func TestAccProjectScoreResource_categoricalDefinition(t *testing.T) {
	name := "tf-acc-score-config-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	resourceName := "braintrust_project_score.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectScoreDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccCategoricalScoreConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "score_type", "categorical"),
					resource.TestCheckResourceAttrSet(resourceName, "categories_json"),
					resource.TestCheckResourceAttrSet(resourceName, "config_json"),
				),
			},
			{ResourceName: resourceName, ImportState: true, ImportStateVerify: true},
			{
				Config: testAccCategoricalScoreConfig(name, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "score_type", "categorical"),
					resource.TestCheckResourceAttrSet(resourceName, "categories_json"),
					resource.TestCheckResourceAttrSet(resourceName, "config_json"),
				),
			},
		},
	})
}

func testAccCategoricalScoreConfig(name string, updated bool) string {
	if updated {
		return fmt.Sprintf(`
resource "braintrust_project" "test" { name = %[1]q }
resource "braintrust_project_score" "test" {
  project_id = braintrust_project.test.id
  name = %[1]q
  score_type = "categorical"
  categories_json = jsonencode([
    { name = "Low", value = 0 },
    { name = "Medium", value = 0.5 },
    { name = "High", value = 1 }
  ])
  config_json = jsonencode({ multi_select = true, object_types = ["project_logs"] })
}
`, name)
	}
	return fmt.Sprintf(`
resource "braintrust_project" "test" { name = %[1]q }
resource "braintrust_project_score" "test" {
  project_id = braintrust_project.test.id
  name = %[1]q
  score_type = "categorical"
  categories_json = jsonencode([
    { name = "Low", value = 0 },
    { name = "High", value = 1 }
  ])
  config_json = jsonencode({ multi_select = false, object_types = ["project_logs"] })
}
`, name)
}

func TestProjectScoreJSONInput(t *testing.T) {
	if _, err := projectScoreJSONInput(types.StringValue("{bad"), "config_json"); err == nil {
		t.Fatal("invalid JSON must fail before an API request")
	}
	if _, err := projectScoreJSONInput(types.StringValue("  null "), "config_json"); err == nil {
		t.Fatal("Braintrust ignores a null score config patch; reject it before an API request")
	}
	raw, err := projectScoreJSONInput(types.StringValue(`{"multi_select":true}`), "config_json")
	if err != nil || !json.Valid(raw) {
		t.Fatalf("valid JSON was rejected: %v", err)
	}
	if !scoreJSONEqual(types.StringValue(`{"a":1,"b":2}`), types.StringValue(`{ "b": 2, "a": 1 }`)) {
		t.Fatal("object key order and whitespace should not change a score definition")
	}
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
		_, err := c.API.GetProjectScoreIdWithResponse(context.Background(), rs.Primary.ID)
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
