package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

func TestAPIKeyRetryLogsDoNotExposeResponseBodies(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			const secret = "secret-from-error-body"
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", JSONContentType)
				if requests == 1 {
					if method == http.MethodPost {
						w.WriteHeader(http.StatusTooManyRequests)
					} else {
						w.WriteHeader(http.StatusInternalServerError)
					}
					_, _ = w.Write([]byte(secret))
					return
				}
				_, _ = w.Write([]byte(`{"id":"00000000-0000-0000-0000-000000000001","name":"test","key":"created-secret"}`))
			}))
			defer server.Close()
			c, err := NewClient(server.URL, "auth-secret", "")
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			ctx := tflogtest.RootLogger(context.Background(), &logs)
			if method == http.MethodPost {
				_, err = c.CreateAPIKey(ctx, CreateAPIKeyRequest{Name: "test"})
			} else {
				_, err = c.API.GetApiKeyIdWithResponse(ctx, "00000000-0000-0000-0000-000000000001")
			}
			if err != nil || requests != 2 || !strings.Contains(logs.String(), "Retrying Braintrust API request") {
				t.Fatalf("retry scenario did not complete: requests=%d err=%v", requests, err)
			}
			for _, value := range []string{secret, "created-secret", "auth-secret"} {
				if strings.Contains(logs.String(), value) {
					t.Fatal("credential exposed in provider logs")
				}
			}
		})
	}
}

func TestCreateAPIKeyRefusesExistingResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", JSONContentType)
		w.Header().Set("x-bt-found-existing", "true")
		_, _ = w.Write([]byte(`{"id":"00000000-0000-0000-0000-000000000001","key":"preexisting-secret"}`))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "auth", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.CreateAPIKey(context.Background(), CreateAPIKeyRequest{Name: "test"})
	if err == nil || result != nil || IsUncertainWriteError(err) {
		t.Fatalf("preexisting key must never be adopted: %v", err)
	}
}
