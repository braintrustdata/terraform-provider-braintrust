package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Run with TF_ACC=1, BRAINTRUST_API_KEY, and BRAINTRUST_TEST_ORG_ID. The
// organization must permit custom retention policies.
func TestAccOrgAutomationResource_retention(t *testing.T) {
	orgID := os.Getenv("BRAINTRUST_TEST_ORG_ID")
	if orgID == "" {
		t.Skip("BRAINTRUST_TEST_ORG_ID is required")
	}
	name := "tf-acc-retention-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	config := func(days int) string {
		return fmt.Sprintf(`
resource "braintrust_org_automation" "test" {
  org_id = %q
  name = %q
  retention_config = {
    object_type = "experiment"
    retention_days = %d
  }
}
`, orgID, name, days)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config(90), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("braintrust_org_automation.test", "org_id", orgID),
				resource.TestCheckResourceAttr("braintrust_org_automation.test", "retention_config.retention_days", "90"),
			)},
			{ResourceName: "braintrust_org_automation.test", ImportState: true, ImportStateVerify: true},
			{Config: config(30), Check: resource.TestCheckResourceAttr("braintrust_org_automation.test", "retention_config.retention_days", "30")},
		},
	})
}
