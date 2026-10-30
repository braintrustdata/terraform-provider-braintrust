package client

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
)

// CheckDataPlane returns an error when the client points at a
// Braintrust-hosted data plane but none of the API key's organizations use it.
// Hosted data planes reject (HTTP 421) requests for objects they don't store,
// but forward control-plane writes, so a misconfigured apply fails partway
// through. Self-hosted URLs are not checked: they don't reject other orgs, and
// are often reached through a different hostname than the org's api_url.
func (c *Client) CheckDataPlane(ctx context.Context) error {
	if !isHostedDataPlane(c.baseURL) {
		return nil
	}
	orgs, err := ListObjects("/v1/organization", func(limit int, startingAfter *string) (*Page[openapi.Organization], error) {
		page, err := c.API.GetOrganizationWithResponse(ctx, &openapi.GetOrganizationParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		return page.JSON200, nil
	}, func(o openapi.Organization) string { return o.Id.String() })
	if err != nil {
		return fmt.Errorf("listing organizations: %w", err)
	}
	return dataPlaneMismatch(c.baseURL, orgs)
}

// dataPlaneMismatch reports an error only when every organization has an
// api_url that differs from baseURL. Organizations without an api_url use the
// default data plane, which the hosted data planes don't reject.
func dataPlaneMismatch(baseURL string, orgs []openapi.Organization) error {
	if len(orgs) == 0 {
		return nil
	}
	var others []string
	for _, org := range orgs {
		apiURL := org.ApiUrl.GetOrEmpty()
		if apiURL == "" || sameURL(apiURL, baseURL) {
			return nil
		}
		others = append(others, fmt.Sprintf("organization %q uses %s", org.Name, apiURL))
	}
	return fmt.Errorf("api_url is %s, but %s. Set api_url or %s to your organization's data plane URL",
		baseURL, strings.Join(others, ", "), EnvAPIURL)
}

func isHostedDataPlane(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "braintrust.dev" || strings.HasSuffix(host, ".braintrust.dev")
}

func sameURL(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) &&
		strings.EqualFold(ua.Host, ub.Host) &&
		strings.TrimRight(ua.Path, "/") == strings.TrimRight(ub.Path, "/")
}
