package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccProjectDataSource_basic(t *testing.T) {
	resourceName := "braintrust_project.test"
	dataSourceName := "data.braintrust_project.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				// Create the project resource, then read it via data source.
				Config: testAccProjectDataSourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Verify the data source returns the same values as the resource.
					resource.TestCheckResourceAttrPair(
						dataSourceName, "id",
						resourceName, "id",
					),
					resource.TestCheckResourceAttrPair(
						dataSourceName, "name",
						resourceName, "name",
					),
					resource.TestCheckResourceAttrPair(
						dataSourceName, "org_id",
						resourceName, "org_id",
					),
					resource.TestCheckResourceAttrPair(
						dataSourceName, "created",
						resourceName, "created",
					),
					resource.TestCheckResourceAttrPair(
						dataSourceName, "user_id",
						resourceName, "user_id",
					),
					// Also verify specific values directly.
					resource.TestCheckResourceAttr(dataSourceName, "name", rName),
				),
			},
		},
	})
}

func TestAccProjectDataSource_byName(t *testing.T) {
	resourceName := "braintrust_project.test"
	dataSourceName := "data.braintrust_project.by_name"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectDataSourceByNameConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						dataSourceName, "id",
						resourceName, "id",
					),
					resource.TestCheckResourceAttrPair(
						dataSourceName, "name",
						resourceName, "name",
					),
					resource.TestCheckResourceAttrPair(
						dataSourceName, "org_id",
						resourceName, "org_id",
					),
					resource.TestCheckResourceAttr(dataSourceName, "name", rName),
				),
			},
		},
	})
}

func testAccProjectDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

data "braintrust_project" "test" {
  id = braintrust_project.test.id
}
`, name)
}

func testAccProjectDataSourceByNameConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

data "braintrust_project" "by_name" {
  name = braintrust_project.test.name
}
`, name)
}
