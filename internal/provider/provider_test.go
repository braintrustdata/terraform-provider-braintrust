package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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

func TestProviderConfigureTerraformVersion(t *testing.T) {
	t.Setenv(client.EnvAPIKey, "")
	t.Setenv(client.EnvAPIURL, "")
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	p := &braintrustProvider{version: "test"}
	var schemaResponse frameworkprovider.SchemaResponse
	p.Schema(ctx, frameworkprovider.SchemaRequest{}, &schemaResponse)
	configWithKey := func(key string) tfsdk.Config {
		return tfsdk.Config{
			Schema: schemaResponse.Schema,
			Raw: tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{
				"api_key": tftypes.NewValue(tftypes.String, key),
				"api_url": tftypes.NewValue(tftypes.String, server.URL),
			}),
		}
	}

	for _, tc := range []struct {
		name    string
		version string
		allowed bool
	}{
		{name: "old patch", version: "1.10.5"},
		{name: "broken initial write-only release", version: "1.11.0"},
		{name: "minimum alpha", version: "1.11.1-alpha1"},
		{name: "minimum release candidate", version: "1.11.1-rc1"},
		{name: "minimum", version: "1.11.1", allowed: true},
		{name: "minimum build metadata", version: "1.11.1+build.1", allowed: true},
		{name: "later patch", version: "1.11.2", allowed: true},
		{name: "later minor", version: "1.12.0", allowed: true},
		{name: "later major", version: "2.0.0", allowed: true},
		{name: "empty"},
		{name: "unknown", version: "unknown"},
		{name: "malformed", version: "1.11.not-a-patch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type configurationCase struct {
				name   string
				config tfsdk.Config
			}
			configs := []configurationCase{
				{name: "valid configuration", config: configWithKey("fixture-key")},
			}
			if !tc.allowed {
				configs = append(configs,
					configurationCase{name: "missing authentication", config: configWithKey("")},
					configurationCase{name: "unreadable configuration", config: tfsdk.Config{}},
				)
			}
			for _, configuration := range configs {
				t.Run(configuration.name, func(t *testing.T) {
					before := requests.Load()
					var resp frameworkprovider.ConfigureResponse
					p.Configure(ctx, frameworkprovider.ConfigureRequest{
						TerraformVersion: tc.version,
						Config:           configuration.config,
					}, &resp)
					if requests.Load() != before {
						t.Fatal("Configure unexpectedly contacted the API")
					}
					if tc.allowed {
						if len(resp.Diagnostics) != 0 {
							t.Fatalf("supported version produced diagnostics: %v", resp.Diagnostics)
						}
						configured, ok := resp.ResourceData.(*client.Client)
						if !ok || configured == nil || resp.DataSourceData != configured || resp.EphemeralResourceData != configured {
							t.Fatal("supported version did not configure a shared client for all resource types")
						}
						return
					}
					if len(resp.Diagnostics) != 1 || !resp.Diagnostics.HasError() {
						t.Fatalf("rejected version should produce exactly one error: %v", resp.Diagnostics)
					}
					detail := resp.Diagnostics[0].Detail()
					if !strings.Contains(detail, "1.11.1") || !strings.Contains(detail, strconv.Quote(tc.version)) {
						t.Fatalf("version diagnostic should identify the minimum and actual version: %s", detail)
					}
					if resp.ResourceData != nil || resp.DataSourceData != nil || resp.EphemeralResourceData != nil {
						t.Fatal("rejected version configured an API client")
					}
				})
			}
		})
	}
}
