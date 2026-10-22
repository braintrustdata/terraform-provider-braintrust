package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccDatasetResource_basic(t *testing.T) {
	resourceName := "braintrust_dataset.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	rNameUpdated := rName + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDatasetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDatasetResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "project_id"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccDatasetResourceConfigUpdated(rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rNameUpdated),
					resource.TestCheckResourceAttr(resourceName, "description", "updated description"),
				),
			},
		},
	})
}

func TestDatasetMetadataInput(t *testing.T) {
	for _, input := range []string{"{bad", `["array"]`, `"string"`} {
		if _, err := datasetMetadataInput(types.StringValue(input)); err == nil {
			t.Errorf("metadata_json %q should be rejected", input)
		}
	}
	if _, err := datasetMetadataInput(types.StringValue(`{"owner":"qa"}`)); err != nil {
		t.Fatalf("valid metadata was rejected: %v", err)
	}
}

func TestAccDatasetResource_definition(t *testing.T) {
	name := "tf-acc-dataset-definition-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	resourceName := "braintrust_dataset.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDatasetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDatasetDefinitionConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "tags.#", "2"),
					resource.TestCheckResourceAttrSet(resourceName, "metadata_json"),
					resource.TestCheckResourceAttrSet(resourceName, "url_slug"),
					resource.TestCheckResourceAttr("data.braintrust_dataset.lookup", "tags.#", "2"),
					resource.TestCheckResourceAttrSet("data.braintrust_dataset.lookup", "metadata_json"),
					resource.TestCheckResourceAttrSet("data.braintrust_dataset.lookup", "url_slug"),
				),
			},
			{ResourceName: resourceName, ImportState: true, ImportStateVerify: true},
			{
				Config: testAccDatasetDefinitionConfig(name, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "tags.#", "0"),
					resource.TestCheckResourceAttr(resourceName, "metadata_json", "null"),
					resource.TestCheckResourceAttr("data.braintrust_dataset.lookup", "tags.#", "0"),
					resource.TestCheckResourceAttr("data.braintrust_dataset.lookup", "metadata_json", "null"),
				),
			},
		},
	})
}

func testAccDatasetDefinitionConfig(name string, cleared bool) string {
	if cleared {
		return fmt.Sprintf(`
resource "braintrust_project" "test" { name = %[1]q }
resource "braintrust_dataset" "test" {
  project_id = braintrust_project.test.id
  name = %[1]q
  tags = []
  metadata_json = "null"
}
data "braintrust_dataset" "lookup" { id = braintrust_dataset.test.id }
`, name)
	}
	return fmt.Sprintf(`
resource "braintrust_project" "test" { name = %[1]q }
resource "braintrust_dataset" "test" {
  project_id = braintrust_project.test.id
  name = %[1]q
  tags = ["gold", "reviewed"]
  metadata_json = jsonencode({ owner = "qa", nested = { tier = 1 } })
}
data "braintrust_dataset" "lookup" { id = braintrust_dataset.test.id }
`, name)
}

func testAccCheckDatasetDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_dataset" {
			continue
		}
		_, err := c.API.GetDatasetIdWithResponse(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust dataset %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFoundError(err) {
			return fmt.Errorf("unexpected error checking dataset %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccDatasetResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_dataset" "test" {
  project_id = braintrust_project.test.id
  name       = %[1]q
}
`, name)
}

func testAccDatasetResourceConfigUpdated(name string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_dataset" "test" {
  project_id  = braintrust_project.test.id
  name        = %[1]q
  description = "updated description"
}
`, name)
}
