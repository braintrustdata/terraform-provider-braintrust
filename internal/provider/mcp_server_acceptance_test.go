package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Run with TF_ACC=1, BRAINTRUST_API_KEY, and BRAINTRUST_TEST_ORG_ID.
func TestAccMCPServerResource_lifecycle(t *testing.T) {
	orgID := os.Getenv("BRAINTRUST_TEST_ORG_ID")
	if orgID == "" {
		t.Skip("BRAINTRUST_TEST_ORG_ID is required")
	}
	name := "tf-acc-mcp-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	config := func(url string) string {
		return fmt.Sprintf(`
resource "braintrust_project" "test" {
  name = %q
}
resource "braintrust_mcp_server" "test" {
  project_id = braintrust_project.test.id
  name = "Test tools"
  url = %q
}
`, name, url)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t); requireSingleAccessibleOrg(t, orgID) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config("https://example.com/mcp"), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("braintrust_mcp_server.test", "id"),
				resource.TestCheckResourceAttrPair("braintrust_mcp_server.test", "project_id", "braintrust_project.test", "id"),
			)},
			{ResourceName: "braintrust_mcp_server.test", ImportState: true, ImportStateVerify: true},
			{Config: config("https://example.com/mcp-v2"), Check: resource.TestCheckResourceAttr("braintrust_mcp_server.test", "url", "https://example.com/mcp-v2")},
		},
	})
}

// Project creation picks the credential's organization. Verify the test token
// cannot select another organization before creating anything.
func requireSingleAccessibleOrg(t *testing.T, expectedID string) {
	t.Helper()
	baseURL := os.Getenv(client.EnvAPIURL)
	if baseURL == "" {
		baseURL = client.DefaultBaseURL
	}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/organization?limit=100", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv(client.EnvAPIKey))
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("organization precheck returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Objects []struct {
			ID string `json:"id"`
		} `json:"objects"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Objects) != 1 || result.Objects[0].ID != expectedID {
		t.Fatalf("test credential must see only organization %s; found %d accessible organizations", expectedID, len(result.Objects))
	}
}
