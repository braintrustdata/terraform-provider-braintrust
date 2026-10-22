package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// doer is the HTTP layer under the generated SDK client. It adds auth, retries,
// logging, and maps unsuccessful responses to Go errors, so callers can treat
// a nil error as a 2xx response.
type doer struct {
	httpClient *http.Client
	apiKey     string
	userAgent  string
}

func (d *doer) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	method := req.Method
	path := req.URL.Path

	tflog.Debug(ctx, "Braintrust API request", map[string]any{
		"method": method,
		"path":   path,
	})

	// Generated operations interpolate IDs into the path, so an empty ID
	// yields a trailing or doubled slash that would hit a different endpoint.
	if strings.HasSuffix(path, "/") || strings.Contains(path, "//") {
		return nil, fmt.Errorf("%s %s: resource id is required", method, path)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if req.Body != nil && req.GetBody == nil {
				break
			}

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
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		attemptReq, err := cloneRequest(req, attempt)
		if err != nil {
			return nil, err
		}
		attemptReq.Header.Set("Authorization", "Bearer "+d.apiKey)
		attemptReq.Header.Set("Accept", "application/json")
		attemptReq.Header.Set("User-Agent", d.userAgent)

		resp, err := d.httpClient.Do(attemptReq)
		if err != nil {
			lastErr = fmt.Errorf("failed to execute request: %w", err)
			// Don't retry if the context is done (canceled or deadline exceeded).
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if method != http.MethodGet {
				return nil, &UncertainWriteError{Method: method, Cause: lastErr}
			}
			continue
		}

		// Read and close the response body immediately (not deferred)
		// to avoid leaking file descriptors across retry iterations.
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
		_ = resp.Body.Close() // intentionally discarding close error
		if err != nil {
			if method != http.MethodGet {
				return nil, &UncertainWriteError{Method: method, Cause: fmt.Errorf("failed to read response body: %w", err)}
			}
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		// Success
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			tflog.Debug(ctx, "Braintrust API request succeeded", map[string]any{
				"method":      method,
				"path":        path,
				"status_code": resp.StatusCode,
				"attempts":    attempt + 1,
			})
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
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
			return nil, &NotFoundError{Message: message}
		}

		// Retriable error
		if retriableStatusCodes[resp.StatusCode] {
			lastErr = &APIError{
				StatusCode: resp.StatusCode,
				Message:    string(respBody),
			}
			if method != http.MethodGet && resp.StatusCode != http.StatusTooManyRequests {
				return nil, &UncertainWriteError{Method: method, Cause: lastErr}
			}
			continue
		}

		// Non-retriable error
		tflog.Error(ctx, "Braintrust API request failed", map[string]any{
			"method":      method,
			"path":        path,
			"status_code": resp.StatusCode,
		})
		return nil, &APIError{
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
		return nil, fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
	}
	return nil, fmt.Errorf("request failed after %d retries", maxRetries)
}

// cloneRequest returns a request that is safe to send for the given attempt.
// Bodies are consumed by sending, so retries need a fresh copy.
func cloneRequest(req *http.Request, attempt int) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if attempt == 0 || req.Body == nil {
		return clone, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("failed to rewind request body: %w", err)
	}
	clone.Body = body
	return clone, nil
}

// Write marks errors from a non-idempotent generated call as uncertain. The
// transport already classifies HTTP failures; this covers responses that were
// received but could not be decoded, where the write may have committed.
func Write[R any](resp R, err error) (R, error) {
	if err != nil && !isClassified(err) {
		err = &UncertainWriteError{Method: "write", Cause: fmt.Errorf("failed to decode response: %w", err)}
	}
	return resp, err
}

func isClassified(err error) bool {
	var apiErr *APIError
	var notFound *NotFoundError
	return IsUncertainWriteError(err) ||
		errors.As(err, &apiErr) ||
		errors.As(err, &notFound) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// RejectExisting rejects Braintrust's idempotent POST response for an existing
// object. Adopting that object as newly created Terraform state would make a
// later destroy delete infrastructure this configuration did not create.
func RejectExisting(resp *http.Response, resourceType string) error {
	if resp != nil && resp.Header.Get("x-bt-found-existing") != "" {
		return fmt.Errorf("%s already exists; import the existing resource instead of creating it", resourceType)
	}
	return nil
}

const listPageSize = 100

// ListAll follows the standard Braintrust list cursor for a generated list
// operation. Callers still apply any filters the endpoint cannot express.
func ListAll[T any](what string, page func(limit int, startingAfter *string) ([]T, error), idOf func(T) string) ([]T, error) {
	var all []T
	var cursor *string
	for {
		objects, err := page(listPageSize, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, objects...)
		if len(objects) < listPageSize {
			return all, nil
		}
		next := idOf(objects[len(objects)-1])
		if next == "" || (cursor != nil && next == *cursor) {
			return nil, fmt.Errorf("%s pagination cursor did not advance", what)
		}
		cursor = &next
	}
}

// JSON returns a generated response's decoded body, or an error when the
// generated client did not decode one (for example, a non-JSON content type).
func JSON[T any](v *T) (*T, error) {
	if v == nil {
		return nil, errors.New("braintrust API returned an unexpected response body")
	}
	return v, nil
}

// Created unwraps a successful generated create response: it rejects objects
// the API reports as already existing and returns the decoded body.
func Created[T any](resp *http.Response, body *T, resourceType string) (*T, error) {
	if err := RejectExisting(resp, resourceType); err != nil {
		return nil, err
	}
	return JSON(body)
}

// WithQuery adds a query parameter to a generated request. It covers filters
// the API accepts but the pinned spec does not declare.
func WithQuery(key, value string) openapi.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		query := req.URL.Query()
		query.Set(key, value)
		req.URL.RawQuery = query.Encode()
		return nil
	}
}
