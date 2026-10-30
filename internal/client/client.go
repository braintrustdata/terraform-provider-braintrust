package client

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
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
	// API is the generated Braintrust SDK client. Its transport returns an
	// error for every non-2xx response, so a nil error means success.
	API *openapi.ClientWithResponses

	baseURL string
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

	api, err := openapi.NewClientWithResponses(baseURL, openapi.WithHTTPClient(&doer{
		httpClient: &http.Client{Timeout: defaultTimeout},
		apiKey:     apiKey,
		userAgent:  userAgent,
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create Braintrust API client: %w", err)
	}
	return &Client{API: api, baseURL: baseURL}, nil
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

// UncertainWriteError means a write may have committed even though its result
// could not be observed. Callers must reconcile before issuing another write.
type UncertainWriteError struct {
	Method string
	Cause  error
}

func (e *UncertainWriteError) Error() string {
	return fmt.Sprintf("%s request outcome is unknown; inspect the remote resource before retrying: %v", e.Method, e.Cause)
}

func (e *UncertainWriteError) Unwrap() error { return e.Cause }

// RequestNotSentError means a request was rejected before it was sent, so a
// write cannot have committed.
type RequestNotSentError struct {
	Cause error
}

func (e *RequestNotSentError) Error() string { return e.Cause.Error() }

func (e *RequestNotSentError) Unwrap() error { return e.Cause }

func IsUncertainWriteError(err error) bool {
	var uncertain *UncertainWriteError
	return errors.As(err, &uncertain)
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
		strings.Contains(strings.ToLower(apiErr.Message), "does not exist") {
		return true
	}
	return false
}
