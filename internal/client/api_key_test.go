package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCreateAPIKeyUncertainResponse(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000001"
	for _, tc := range []struct {
		name        string
		body        string
		contentType string
		wantID      bool
	}{
		{"missing key", `{"id":"` + id + `"}`, JSONContentType, true},
		{"missing id", `{"key":"secret-value"}`, JSONContentType, false},
		{"invalid id", `{"id":"secret-value","key":"secret-value"}`, JSONContentType, false},
		{"invalid expiry", `{"id":"` + id + `","expires_at":"secret-value","key":"secret-value"}`, JSONContentType, true},
		{"wrong content type", `{"id":"` + id + `","key":"secret-value"}`, "text/html", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			c, err := NewClient(server.URL, "auth", "")
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.CreateAPIKey(context.Background(), CreateAPIKeyRequest{Name: "test"})
			if !IsUncertainWriteError(err) || requests != 1 {
				t.Fatalf("expected one uncertain write, got requests=%d err=%v", requests, err)
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatal("decode error exposed key material")
			}
			if tc.wantID && (result == nil || result.Id != uuid.MustParse(id)) {
				t.Fatal("lost response ID needed for safe reconciliation")
			}
		})
	}
}

func TestCreateAPIKeyHTTPFailures(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(status)
			}))
			defer server.Close()
			c, err := NewClient(server.URL, "auth", "")
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.CreateAPIKey(context.Background(), CreateAPIKeyRequest{Name: "test"})
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != status || requests != 1 || IsUncertainWriteError(err) != (status == http.StatusInternalServerError) {
				t.Fatalf("incorrect write classification/retry: requests=%d err=%v", requests, err)
			}
		})
	}
}
