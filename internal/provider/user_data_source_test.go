package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUserDataSource_byEmail(t *testing.T) {
	// This test requires a real user email that exists in the org.
	// Set BRAINTRUST_TEST_USER_EMAIL to a valid email before running.
	testEmail := os.Getenv("BRAINTRUST_TEST_USER_EMAIL")
	if testEmail == "" {
		t.Skip("Set BRAINTRUST_TEST_USER_EMAIL to run this test")
	}

	dataSourceName := "data.braintrust_user.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserDataSourceConfigByEmail(testEmail),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(dataSourceName, "id"),
					resource.TestCheckResourceAttr(dataSourceName, "email", testEmail),
				),
			},
		},
	})
}

func TestAccUserDataSource_byID(t *testing.T) {
	// This test requires both a valid user email and its known ID.
	testEmail := os.Getenv("BRAINTRUST_TEST_USER_EMAIL")
	testID := os.Getenv("BRAINTRUST_TEST_USER_ID")
	if testEmail == "" || testID == "" {
		t.Skip("Set BRAINTRUST_TEST_USER_EMAIL and BRAINTRUST_TEST_USER_ID to run this test")
	}

	dataSourceName := "data.braintrust_user.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserDataSourceConfigByID(testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "id", testID),
					resource.TestCheckResourceAttr(dataSourceName, "email", testEmail),
				),
			},
		},
	})
}

func testAccUserDataSourceConfigByEmail(email string) string {
	return fmt.Sprintf(`
data "braintrust_user" "test" {
  email = %[1]q
}
`, email)
}

func testAccUserDataSourceConfigByID(id string) string {
	return fmt.Sprintf(`
data "braintrust_user" "test" {
  id = %[1]q
}
`, id)
}
