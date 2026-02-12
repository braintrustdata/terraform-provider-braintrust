package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDatasetDataSource_basic(t *testing.T) {
	resourceName := "braintrust_dataset.test"
	dataSourceName := "data.braintrust_dataset.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDatasetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDatasetDataSourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "id", resourceName, "id"),
					resource.TestCheckResourceAttrPair(dataSourceName, "name", resourceName, "name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "project_id", resourceName, "project_id"),
				),
			},
		},
	})
}

func TestAccDatasetDataSource_byName(t *testing.T) {
	resourceName := "braintrust_dataset.test"
	dataSourceName := "data.braintrust_dataset.by_name"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDatasetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDatasetDataSourceByNameConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "id", resourceName, "id"),
					resource.TestCheckResourceAttrPair(dataSourceName, "name", resourceName, "name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "project_id", resourceName, "project_id"),
				),
			},
		},
	})
}

func testAccDatasetDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_dataset" "test" {
  project_id = braintrust_project.test.id
  name       = %[1]q
}

data "braintrust_dataset" "test" {
  id = braintrust_dataset.test.id
}
`, name)
}

func testAccDatasetDataSourceByNameConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_dataset" "test" {
  project_id = braintrust_project.test.id
  name       = %[1]q
}

data "braintrust_dataset" "by_name" {
  project_id = braintrust_project.test.id
  name       = braintrust_dataset.test.name
}
`, name)
}
