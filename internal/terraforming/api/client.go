// Package api lists Braintrust resources for the exporter. It reads through
// the Braintrust Go SDK client shared with the provider.
package api

import (
	"strings"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
)

// DefaultBaseURL is the default Braintrust API base URL.
const DefaultBaseURL = client.DefaultBaseURL

// Client reads Braintrust resources for export.
type Client struct {
	sdk *client.Client
}

// NewClient creates an exporter client.
func NewClient(baseURL, apiKey, userAgent string) (*Client, error) {
	sdk, err := client.NewClient(baseURL, apiKey, userAgent)
	if err != nil {
		return nil, err
	}
	return &Client{sdk: sdk}, nil
}

// SanitizeName converts a resource name into a safe Terraform identifier.
func SanitizeName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '/' || r == '.':
			b.WriteRune('_')
		}
	}
	return b.String()
}
