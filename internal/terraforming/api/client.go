package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the default Braintrust API base URL.
	DefaultBaseURL = "https://api.braintrust.dev"

	defaultUserAgent    = "braintrust-terraforming"
	defaultTimeout      = 30 * time.Second
	maxRetries          = 3
	maxResponseBodySize = 10 << 20
	maxJitter           = time.Second
)

// retriableStatusCodes are HTTP status codes that should trigger a retry.
var retriableStatusCodes = map[int]bool{
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

// Client is the Braintrust API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	userAgent  string
	Verbose    bool
}

// NewClient creates a new Braintrust API client.
func NewClient(baseURL, apiKey, userAgent string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("api_key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		userAgent: userAgent,
	}, nil
}

// APIError represents an error returned by the Braintrust API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("braintrust API error (status %d): %s", e.StatusCode, e.Message)
}

// doRequest performs an HTTP request with retry logic and returns the result.
func (c *Client) doRequest(ctx context.Context, method, path string, body any, result any) error {
	rawURL := fmt.Sprintf("%s%s", c.baseURL, path)

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1))*time.Second +
				time.Duration(rand.Int64N(int64(maxJitter)))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		var bodyReader io.Reader
		if body != nil {
			jsonBody, err := json.Marshal(body)
			if err != nil {
				return fmt.Errorf("failed to marshal request body: %w", err)
			}
			bodyReader = bytes.NewReader(jsonBody)
		}

		req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to execute request: %w", err)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if result != nil && len(respBody) > 0 {
				if err := json.Unmarshal(respBody, result); err != nil {
					return fmt.Errorf("failed to unmarshal response: %w", err)
				}
			}
			return nil
		}

		if resp.StatusCode == http.StatusNotFound {
			message := string(respBody)
			if message == "" {
				message = "not found"
			}
			return &APIError{StatusCode: resp.StatusCode, Message: message}
		}

		if retriableStatusCodes[resp.StatusCode] {
			lastErr = &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
			continue
		}

		return &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	if lastErr != nil {
		return fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
	}
	return fmt.Errorf("request failed after %d retries", maxRetries)
}

// listResponse is the generic wrapper for paginated list endpoints.
type listResponse struct {
	Objects []json.RawMessage `json:"objects"`
}

// ListAll fetches all objects from a paginated list endpoint.
// It handles cursor-based pagination automatically.
func (c *Client) ListAll(ctx context.Context, basePath string, params url.Values) ([]json.RawMessage, error) {
	var all []json.RawMessage
	if params == nil {
		params = url.Values{}
	} else {
		copied := url.Values{}
		for key, values := range params {
			copied[key] = append([]string(nil), values...)
		}
		params = copied
	}
	const pageSize = 100
	params.Set("limit", "100")
	var previousCursor string

	for {
		path := basePath
		if len(params) > 0 {
			path += "?" + params.Encode()
		}

		var resp listResponse
		if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
			return nil, err
		}

		all = append(all, resp.Objects...)

		// If we got fewer objects than expected or none, we're done.
		// The Braintrust API uses cursor-based pagination with starting_after.
		if len(resp.Objects) == 0 {
			break
		}

		// Extract the ID of the last object for cursor pagination.
		var lastObj struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(resp.Objects[len(resp.Objects)-1], &lastObj); err != nil {
			return nil, fmt.Errorf("failed to extract cursor from last object: %w", err)
		}
		if lastObj.ID == "" {
			return nil, fmt.Errorf("missing pagination cursor from %s", basePath)
		}

		// Check if we got a full page. Default limit is 100.
		// If we got fewer, there are no more pages.
		if len(resp.Objects) < pageSize {
			break
		}
		if lastObj.ID == previousCursor {
			return nil, fmt.Errorf("pagination cursor did not advance for %s", basePath)
		}
		previousCursor = lastObj.ID

		params.Set("starting_after", lastObj.ID)
	}

	return all, nil
}

// GetBaseURL returns the configured base URL.
func (c *Client) GetBaseURL() string {
	return c.baseURL
}

// unmarshalAll decodes a slice of json.RawMessage into a typed slice.
func UnmarshalAll[T any](raw []json.RawMessage) ([]T, error) {
	result := make([]T, 0, len(raw))
	for _, r := range raw {
		var v T
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, fmt.Errorf("failed to unmarshal object: %w", err)
		}
		result = append(result, v)
	}
	return result, nil
}

// FetchJSON fetches a URL and returns the raw bytes. Used for downloading specs.
func FetchJSON(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
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
