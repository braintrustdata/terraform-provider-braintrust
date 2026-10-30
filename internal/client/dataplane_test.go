package client

import (
	"strings"
	"testing"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/oapi-codegen/nullable"
)

func org(name, apiURL string) openapi.Organization {
	o := openapi.Organization{Name: name}
	if apiURL != "" {
		o.ApiUrl = nullable.NewNullableWithValue(apiURL)
	}
	return o
}

func TestDataPlaneMismatch(t *testing.T) {
	const hosted = "https://api.braintrust.dev"
	for _, tc := range []struct {
		name    string
		baseURL string
		orgs    []openapi.Organization
		wantErr string
	}{
		{name: "no orgs", baseURL: hosted},
		{name: "default data plane", baseURL: hosted, orgs: []openapi.Organization{org("acme", "")}},
		{name: "matching api_url", baseURL: hosted + "/", orgs: []openapi.Organization{org("acme", "HTTPS://API.braintrust.dev")}},
		{name: "one of several orgs matches", baseURL: hosted, orgs: []openapi.Organization{org("eu", "https://api-eu.braintrust.dev"), org("acme", "")}},
		{
			name:    "org on another data plane",
			baseURL: hosted,
			orgs:    []openapi.Organization{org("eu", "https://api-eu.braintrust.dev")},
			wantErr: `api_url is https://api.braintrust.dev, but organization "eu" uses https://api-eu.braintrust.dev`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := dataPlaneMismatch(tc.baseURL, tc.orgs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestIsHostedDataPlane(t *testing.T) {
	for url, want := range map[string]bool{
		"https://api.braintrust.dev":         true,
		"https://api-eu.braintrust.dev/":     true,
		"https://braintrust.dev":             true,
		"https://bt.example.com":             false,
		"https://braintrust.dev.example.com": false,
		"http://localhost:8000":              false,
	} {
		if got := isHostedDataPlane(url); got != want {
			t.Errorf("isHostedDataPlane(%q) = %v, want %v", url, got, want)
		}
	}
}
