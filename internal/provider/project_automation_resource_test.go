package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccProjectAutomationResource_retention tests the simplest config variant
// (retention) which requires no external services.
func TestAccProjectAutomationResource_retention(t *testing.T) {
	resourceName := "braintrust_project_automation.test"
	projName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	autoName := "tf-acc-retention-" + acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectAutomationDestroy,
		Steps: []resource.TestStep{
			// Step 1: Create retention automation.
			{
				Config: testAccProjectAutomationRetentionConfig(projName, autoName, 90),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", autoName),
					resource.TestCheckResourceAttr(resourceName, "retention_config.object_type", "project_logs"),
					resource.TestCheckResourceAttr(resourceName, "retention_config.retention_days", "90"),
					resource.TestCheckResourceAttrSet(resourceName, "project_id"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
				),
			},
			// Step 2: Import.
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Step 3: Update retention_days.
			{
				Config: testAccProjectAutomationRetentionConfig(projName, autoName, 30),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "retention_config.retention_days", "30"),
				),
			},
		},
	})
}

// TestAccProjectAutomationResource_logsWebhook tests the logs + webhook config.
func TestAccProjectAutomationResource_logsWebhook(t *testing.T) {
	resourceName := "braintrust_project_automation.test"
	projName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	autoName := "tf-acc-webhook-" + acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectAutomationDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectAutomationLogsWebhookConfig(projName, autoName, "https://hooks.example.com/test1", 300),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", autoName),
					resource.TestCheckResourceAttr(resourceName, "logs_config.btql_filter", "scores.accuracy < 0.8"),
					resource.TestCheckResourceAttr(resourceName, "logs_config.interval_seconds", "300"),
					resource.TestCheckResourceAttr(resourceName, "logs_config.webhook_action.url", "https://hooks.example.com/test1"),
				),
			},
			// Import.
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: change webhook URL and interval.
			{
				Config: testAccProjectAutomationLogsWebhookConfig(projName, autoName, "https://hooks.example.com/test2", 600),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "logs_config.webhook_action.url", "https://hooks.example.com/test2"),
					resource.TestCheckResourceAttr(resourceName, "logs_config.interval_seconds", "600"),
				),
			},
		},
	})
}

func TestAccProjectAutomationResource_environmentUpdateWebhook(t *testing.T) {
	resourceName := "braintrust_project_automation.test"
	projName := "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	autoName := "tf-acc-environment-" + acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectAutomationDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectAutomationEnvironmentConfig(projName, autoName, "staging"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "environment_update_config.environment_filter.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "environment_update_config.webhook_action.url", "https://hooks.example.com/terraform-test"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccProjectAutomationEnvironmentConfig(projName, autoName, "production"),
				Check:  resource.TestCheckResourceAttr(resourceName, "environment_update_config.environment_filter.0", "production"),
			},
		},
	})
}

func TestAccProjectAutomationResource_rawConfig(t *testing.T) {
	orgID := os.Getenv("BRAINTRUST_TEST_ORG_ID")
	if orgID == "" {
		t.Skip("BRAINTRUST_TEST_ORG_ID is required")
	}
	projName := "tf-acc-raw-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	autoName := "tf-acc-raw-retention-" + acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)
	config := func(days int) string {
		return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %q
}
resource "braintrust_project_automation" "test" {
  project_id = braintrust_project.test.id
  name = %q
  config_json = jsonencode({
    event_type = "retention"
    object_type = "project_logs"
    retention_days = %d
  })
}
`, projName, autoName, days)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t); requireSingleAccessibleOrg(t, orgID) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectAutomationDestroy,
		Steps: []resource.TestStep{
			{Config: config(90), Check: resource.TestCheckResourceAttrSet("braintrust_project_automation.test", "id")},
			{Config: config(30), Check: resource.TestCheckResourceAttrSet("braintrust_project_automation.test", "config_json")},
		},
	})
}

// ---------------------------------------------------------------------------
// Config helpers
// ---------------------------------------------------------------------------

func testAccProjectAutomationRetentionConfig(projName, autoName string, retentionDays int) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_project_automation" "test" {
  project_id = braintrust_project.test.id
  name       = %[2]q

  retention_config = {
    object_type    = "project_logs"
    retention_days = %[3]d
  }
}
`, projName, autoName, retentionDays)
}

func testAccProjectAutomationLogsWebhookConfig(projName, autoName, webhookURL string, interval int) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_project_automation" "test" {
  project_id = braintrust_project.test.id
  name       = %[2]q

  logs_config = {
    btql_filter      = "scores.accuracy < 0.8"
    interval_seconds = %[4]d

    webhook_action = {
      url = %[3]q
    }
  }
}
`, projName, autoName, webhookURL, interval)
}

func testAccProjectAutomationEnvironmentConfig(projName, autoName, environment string) string {
	return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %[1]q
}

resource "braintrust_project_automation" "test" {
  project_id = braintrust_project.test.id
  name       = %[2]q
  environment_update_config = {
    environment_filter = [%[3]q]
    webhook_action = {
      url = "https://hooks.example.com/terraform-test"
    }
  }
}
`, projName, autoName, environment)
}

// ---------------------------------------------------------------------------
// Destroy check
// ---------------------------------------------------------------------------

func testAccCheckProjectAutomationDestroy(s *terraform.State) error {
	c, err := client.NewClient(os.Getenv(client.EnvAPIURL), os.Getenv(client.EnvAPIKey), "")
	if err != nil {
		return fmt.Errorf("failed to create client for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "braintrust_project_automation" {
			continue
		}
		_, err := c.GetProjectAutomation(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("braintrust_project_automation %s still exists", rs.Primary.ID)
		}
	}
	return nil
}
