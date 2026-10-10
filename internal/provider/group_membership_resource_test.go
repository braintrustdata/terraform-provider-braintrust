package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccGroupMembershipResource_basic(t *testing.T) {
	userID := os.Getenv("BRAINTRUST_TEST_USER_ID")
	if userID == "" {
		t.Skip("Set BRAINTRUST_TEST_USER_ID to run this test")
	}
	name := "tf-acc-member-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	config := fmt.Sprintf(`
resource "braintrust_group" "test" {
  name = %q
}

resource "braintrust_group_membership" "test" {
  group_id = braintrust_group.test.id
  user_id  = %q
}
`, name, userID)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("braintrust_group_membership.test", "group_id", "braintrust_group.test", "id"),
					resource.TestCheckResourceAttr("braintrust_group_membership.test", "user_id", userID),
				),
			},
			{
				ResourceName:      "braintrust_group_membership.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
