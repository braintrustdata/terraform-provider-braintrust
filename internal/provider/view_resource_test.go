package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestViewJSONRemovesKeys(t *testing.T) {
	if !viewJSONRemovesKeys(`{"viewType":"monitor","options":{"projectId":"x","rangeValue":"7d"}}`, `{"viewType":"monitor","options":{"projectId":"x"}}`) {
		t.Fatal("nested option removal must be detected")
	}
	if viewJSONRemovesKeys(`{"viewType":"monitor","options":{"projectId":"x"}}`, `{"viewType":"monitor","options":{"projectId":"x","rangeValue":"7d"}}`) {
		t.Fatal("adding an option is supported")
	}
}

func TestAccViewResource_orgProjectDashboard(t *testing.T) {
	orgID := os.Getenv("BRAINTRUST_TEST_ORG_ID")
	if orgID == "" {
		t.Skip("BRAINTRUST_TEST_ORG_ID is required")
	}
	projectName := "tf-org-dashboard-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	config := fmt.Sprintf(`
resource "braintrust_project" "view_test" { name = %[1]q }
resource "braintrust_view" "org_dashboard" {
  object_type = "org_project"
  object_id   = %[2]q
  view_type   = "monitor"
  name        = "Terraform org-scoped dashboard"
  options_json = jsonencode({ viewType = "monitor", options = { projectId = braintrust_project.view_test.id, spanType = "range", rangeValue = "7d" } })
  view_data_json = jsonencode({ custom_charts = { version = "0.0.0", layout = { type = "linear", order = ["note"] }, charts = { note = { type = "note", markdown = "## Org dashboard" } } } })
}
`, projectName, orgID)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("braintrust_view.org_dashboard", "object_type", "org_project"),
				resource.TestCheckResourceAttrSet("braintrust_view.org_dashboard", "id"),
			)},
			{ResourceName: "braintrust_view.org_dashboard", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: viewImportID("braintrust_view.org_dashboard")},
		},
	})
}

func TestAccViewResource_dashboardAndSavedView(t *testing.T) {
	projectName := "tf-view-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	dashboardName := "tf-dashboard-" + acctest.RandStringFromCharSet(7, acctest.CharSetAlphaNum)
	savedName := "tf-saved-" + acctest.RandStringFromCharSet(7, acctest.CharSetAlphaNum)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "terraform-provider-braintrust/acceptance")
			if err != nil {
				return err
			}
			for _, resource := range s.RootModule().Resources {
				if resource.Type != "braintrust_view" {
					continue
				}
				_, err := getView(context.Background(), c, resource.Primary.ID, resource.Primary.Attributes["object_type"], resource.Primary.Attributes["object_id"])
				if !client.IsNotFoundError(err) {
					return fmt.Errorf("view %s still exists or could not be checked: %v", resource.Primary.ID, err)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: viewTestConfig(projectName, dashboardName, savedName, false), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("braintrust_view.dashboard", "view_type", "monitor"),
				resource.TestCheckResourceAttr("braintrust_view.dashboard", "starred", "true"),
				resource.TestCheckResourceAttrSet("braintrust_view.dashboard", "id"),
				resource.TestCheckResourceAttr("braintrust_view.saved", "view_type", "logs"),
			)},
			{ResourceName: "braintrust_view.dashboard", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: viewImportID("braintrust_view.dashboard")},
			{ResourceName: "braintrust_view.saved", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: viewImportID("braintrust_view.saved")},
			{Config: viewTestConfig(projectName, dashboardName, savedName, true), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("braintrust_view.dashboard", "starred", "false"),
				resource.TestCheckResourceAttr("braintrust_view.dashboard", "name", dashboardName+"-updated"),
			)},
			{Config: viewTestConfig(projectName, dashboardName, savedName, false), Check: resource.TestCheckResourceAttrWith("braintrust_view.dashboard", "view_data_json", func(value string) error {
				if strings.Contains(value, `"total"`) {
					return fmt.Errorf("removed chart remains in dashboard")
				}
				return nil
			})},
		},
	})
}

func viewImportID(name string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		r := s.RootModule().Resources[name]
		if r == nil {
			return "", fmt.Errorf("resource %s not found", name)
		}
		return r.Primary.Attributes["object_type"] + "/" + r.Primary.Attributes["object_id"] + "/" + r.Primary.ID, nil
	}
}

func viewTestConfig(projectName, dashboardName, savedName string, update bool) string {
	starred := "true"
	rangeValue := "7d"
	description := "Initial dashboard"
	chart := `note = { type = "note", markdown = "## Test dashboard" }`
	if update {
		starred = "false"
		rangeValue = "30d"
		description = "Updated dashboard"
		dashboardName += "-updated"
		chart = `note = { type = "note", markdown = "## Updated dashboard" }
      total = { title = "Total spans", chartSql = format("SELECT count(1) AS spans FROM project_logs('%s')", braintrust_project.view_test.id), viz = { type = "singleValue", unitType = "count" } }`
	}
	return fmt.Sprintf(`
resource "braintrust_project" "view_test" { name = %[1]q }
resource "braintrust_view" "dashboard" {
  object_type = "project"
  object_id   = braintrust_project.view_test.id
  view_type   = "monitor"
  name        = %[2]q
  description = %[8]q
  starred     = %[4]s
  options_json = jsonencode({ viewType = "monitor", options = { spanType = "range", rangeValue = %[5]q } })
  view_data_json = jsonencode({ custom_charts = {
    version = "0.0.0"
    layout = { type = "linear", order = [%[6]s] }
    charts = { %[7]s }
  } })
}
resource "braintrust_view" "saved" {
  object_type = "project"
  object_id   = braintrust_project.view_test.id
  view_type   = "logs"
  name        = %[3]q
  view_data_json = jsonencode({ search = { filter = [] } })
}
`, projectName, dashboardName, savedName, starred, rangeValue, map[bool]string{false: `"note"`, true: `"note", "total"`}[update], chart, description)
}
