package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccServiceAccountResource(t *testing.T) {
	resourceName := "braintrust_service_account.test"
	name := fmt.Sprintf("tf-acc-service-account-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccServiceAccountConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
					resource.TestMatchResourceAttr(resourceName, "email", regexp.MustCompile(".+")),
				),
			},
		},
	})
}

func testAccServiceAccountConfig(name string) string {
	return fmt.Sprintf(`
resource "braintrust_service_account" "test" {
  name = %[1]q
}
`, name)
}
