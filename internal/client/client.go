package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	// DefaultBaseURL is the default Braintrust API base URL.
	DefaultBaseURL = "https://api.braintrust.dev"

	defaultUserAgent = "terraform-provider-braintrust"
	defaultTimeout   = 30 * time.Second
	maxRetries       = 3

	// maxResponseBodySize limits response body reads to 10 MB to prevent OOM
	// from misbehaving servers.
	maxResponseBodySize = 10 << 20

	// maxJitter is the upper bound for random jitter added to backoff.
	maxJitter = time.Second

	// EnvAPIKey is the environment variable name for the Braintrust API key.
	EnvAPIKey = "BRAINTRUST_API_KEY"

	// EnvAPIURL is the environment variable name for the Braintrust API URL.
	EnvAPIURL = "BRAINTRUST_API_URL"
)

// retriableStatusCodes are HTTP status codes that should trigger a retry.
var retriableStatusCodes = map[int]bool{
	http.StatusTooManyRequests:     true, // 429
	http.StatusInternalServerError: true, // 500
	http.StatusBadGateway:          true, // 502
	http.StatusServiceUnavailable:  true, // 503
	http.StatusGatewayTimeout:      true, // 504
}

// Client is the Braintrust API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	userAgent  string
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

// NotFoundError represents a 404 error from the API.
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("resource not found: %s", e.Message)
}

// IsNotFoundError checks if an error is a NotFoundError or a 403/400 that
// indicates the resource does not exist. The Braintrust API returns 403
// (not 404) for deleted resources because ACLs are cleaned up on deletion,
// and 400 for certain resource types (e.g., ACLs). Both include a message
// containing "does not exist".
func IsNotFoundError(err error) bool {
	var nfe *NotFoundError
	if errors.As(err, &nfe) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) &&
		(apiErr.StatusCode == http.StatusForbidden || apiErr.StatusCode == http.StatusBadRequest) &&
		strings.Contains(apiErr.Message, "does not exist") {
		return true
	}
	return false
}

// doRequest performs an HTTP request with retry logic and returns the result.
// It retries on network errors and retriable HTTP status codes with exponential
// backoff plus random jitter to avoid thundering-herd effects.
func (c *Client) doRequest(ctx context.Context, method, path string, body any, result any) error {
	url := fmt.Sprintf("%s%s", c.baseURL, path)

	tflog.Debug(ctx, "Braintrust API request", map[string]any{
		"method": method,
		"path":   path,
	})

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter: (1s, 2s, 4s) + rand(0, 1s)
			backoff := time.Duration(1<<uint(attempt-1))*time.Second +
				time.Duration(rand.Int64N(int64(maxJitter)))

			tflog.Debug(ctx, "Retrying Braintrust API request", map[string]any{
				"method":  method,
				"path":    path,
				"attempt": attempt,
				"backoff": backoff.String(),
				"error":   lastErr.Error(),
			})

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		// Create body reader for each attempt.
		var bodyReader io.Reader
		if body != nil {
			jsonBody, err := json.Marshal(body)
			if err != nil {
				return fmt.Errorf("failed to marshal request body: %w", err)
			}
			bodyReader = bytes.NewReader(jsonBody)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
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
			// Don't retry if the context is done (cancelled or deadline exceeded).
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		// Read and close the response body immediately (not deferred)
		// to avoid leaking file descriptors across retry iterations.
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
		_ = resp.Body.Close() // intentionally discarding close error
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		// Success
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			tflog.Debug(ctx, "Braintrust API request succeeded", map[string]any{
				"method":      method,
				"path":        path,
				"status_code": resp.StatusCode,
				"attempts":    attempt + 1,
			})
			if result != nil && len(respBody) > 0 {
				if err := json.Unmarshal(respBody, result); err != nil {
					return fmt.Errorf("failed to unmarshal response: %w", err)
				}
			}
			return nil
		}

		// Not Found - don't retry
		if resp.StatusCode == http.StatusNotFound {
			tflog.Debug(ctx, "Braintrust API resource not found", map[string]any{
				"method":      method,
				"path":        path,
				"status_code": resp.StatusCode,
			})
			message := string(respBody)
			if message == "" {
				message = "not found"
			}
			return &NotFoundError{Message: message}
		}

		// Retriable error
		if retriableStatusCodes[resp.StatusCode] {
			lastErr = &APIError{
				StatusCode: resp.StatusCode,
				Message:    string(respBody),
			}
			continue
		}

		// Non-retriable error
		tflog.Error(ctx, "Braintrust API request failed", map[string]any{
			"method":      method,
			"path":        path,
			"status_code": resp.StatusCode,
		})
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
		}
	}

	tflog.Error(ctx, "Braintrust API request failed after retries", map[string]any{
		"method":      method,
		"path":        path,
		"max_retries": maxRetries,
	})

	if lastErr != nil {
		return fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
	}
	return fmt.Errorf("request failed after %d retries", maxRetries)
}
