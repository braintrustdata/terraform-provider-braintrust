package provider

import (
	"os"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories is used in all acceptance tests to
// instantiate the provider during the test. The "braintrust" key must
// match the provider name used in Terraform configurations.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"braintrust": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck validates that required environment variables are set
// before running acceptance tests.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if v := os.Getenv(client.EnvAPIKey); v == "" {
		t.Fatalf("%s must be set for acceptance tests", client.EnvAPIKey)
	}
}
