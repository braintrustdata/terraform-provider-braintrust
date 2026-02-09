package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccGroupDataSource_basic(t *testing.T) {
	resourceName := "braintrust_group.test"
	dataSourceName := "data.braintrust_group.test"
	rName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGroupDataSourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "id", resourceName, "id"),
					resource.TestCheckResourceAttrPair(dataSourceName, "name", resourceName, "name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "org_id", resourceName, "org_id"),
					resource.TestCheckResourceAttr(dataSourceName, "name", rName),
				),
			},
		},
	})
}

func testAccGroupDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_group" "test" {
  name = %[1]q
}

data "braintrust_group" "test" {
  name = braintrust_group.test.name
}
`, name)
}
