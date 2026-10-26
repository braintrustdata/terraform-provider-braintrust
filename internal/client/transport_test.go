package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
)

const fixtureProject = `{"id":"00000000-0000-0000-0000-000000000001","name":"fixture","org_id":"00000000-0000-0000-0000-000000000002"}`

func TestGeneratedWriteServerErrorIsUncertain(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Write(c.API.PostProjectWithResponse(context.Background(), openapi.CreateProject{Name: "fixture"}))
	var apiErr *APIError
	if !IsUncertainWriteError(err) || !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError || requests != 1 {
		t.Fatalf("want one uncertain write with HTTP 500; got requests=%d error=%v", requests, err)
	}
}

func TestGeneratedWriteUndecodableResponseIsUncertain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"not-a-uuid"}`))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Write(c.API.PostProjectWithResponse(context.Background(), openapi.CreateProject{Name: "fixture"}))
	if !IsUncertainWriteError(err) {
		t.Fatalf("a committed write with an unreadable response must be uncertain; got %v", err)
	}
}

func TestGeneratedWriteRetriesRateLimitWithBody(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixtureProject))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Write(c.API.PostProjectWithResponse(context.Background(), openapi.CreateProject{Name: "fixture"}))
	if err != nil || resp.JSON200 == nil || resp.JSON200.Name != "fixture" {
		t.Fatalf("want rate-limited write to succeed on retry; got %v", err)
	}
	if len(bodies) != 2 || bodies[0] == "" || bodies[0] != bodies[1] {
		t.Fatalf("retry must resend the same body; got %q", bodies)
	}
}

func TestGeneratedReadRetriesAndAuthenticates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer fixture" || r.Header.Get("User-Agent") != "ua" {
			t.Errorf("missing auth or user agent: %v", r.Header)
		}
		if requests == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixtureProject))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "ua")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.API.GetProjectIdWithResponse(context.Background(), "00000000-0000-0000-0000-000000000001")
	if err != nil || requests != 2 || resp.JSON200 == nil {
		t.Fatalf("want successful second read; got requests=%d error=%v", requests, err)
	}
}

func TestGeneratedNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"project does not exist"}`))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.API.GetProjectIdWithResponse(context.Background(), "missing")
	if !IsNotFoundError(err) {
		t.Fatalf("want not found; got %v", err)
	}
}

func TestGeneratedEmptyIDIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request with empty id must not be sent: %s", r.URL.Path)
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.API.GetProjectIdWithResponse(context.Background(), ""); err == nil {
		t.Fatal("want an error for an empty id")
	}
}

func TestGeneratedWriteDroppedResponseIsUncertain(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack response: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Write(c.API.PostProjectWithResponse(context.Background(), openapi.CreateProject{Name: "fixture"}))
	if !IsUncertainWriteError(err) || requests != 1 {
		t.Fatalf("want one uncertain write; got requests=%d error=%v", requests, err)
	}
}

func TestCreatedRejectsExistingObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-bt-found-existing", "true")
		_, _ = w.Write([]byte(fixtureProject))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Write(c.API.PostProjectWithResponse(context.Background(), openapi.CreateProject{Name: "fixture"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Created(resp.HTTPResponse, resp.JSON200, "project"); err == nil || !strings.Contains(err.Error(), "import") {
		t.Fatalf("creating an existing project must require import; got %v", err)
	}
}

func TestCreatedUndecodedBodyIsUncertain(t *testing.T) {
	for _, test := range []struct {
		name        string
		status      int
		contentType string
	}{
		{"non-200 success", http.StatusCreated, "application/json"},
		{"non-JSON content type", http.StatusOK, "text/plain"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(fixtureProject))
			}))
			defer server.Close()
			c, err := NewClient(server.URL, "fixture", "")
			if err != nil {
				t.Fatal(err)
			}
			resp, err := Write(c.API.PostProjectWithResponse(context.Background(), openapi.CreateProject{Name: "fixture"}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Created(resp.HTTPResponse, resp.JSON200, "project"); !IsUncertainWriteError(err) {
				t.Fatalf("a committed create without a decoded body must be uncertain; got %v", err)
			}
			if _, err := Written(resp.JSON200); !IsUncertainWriteError(err) {
				t.Fatalf("a committed write without a decoded body must be uncertain; got %v", err)
			}
		})
	}
}

func TestWrittenRawUndecodableFieldsAreUncertain(t *testing.T) {
	project := openapi.Project{Name: "fixture"}
	if _, err := WrittenRaw(&project, []byte("not json")); !IsUncertainWriteError(err) {
		t.Fatalf("want an uncertain write; got %v", err)
	}
	if raw, err := WrittenRaw(&project, []byte(`{"name":"fixture"}`)); err != nil || raw.Value != &project {
		t.Fatalf("want the decoded body; got %v, %v", raw, err)
	}
}

func TestWriteNotSentIsNotUncertain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request with empty id must not be sent: %s", r.URL.Path)
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Write(c.API.DeleteProjectIdWithResponse(context.Background(), ""))
	var notSent *RequestNotSentError
	if !errors.As(err, &notSent) || IsUncertainWriteError(err) {
		t.Fatalf("an unsent write must not be uncertain; got %v", err)
	}
}

func TestIsNotFoundErrorIgnoresCase(t *testing.T) {
	for _, message := range []string{"object does not exist", "Object Does Not Exist", "DOES NOT EXIST"} {
		if !IsNotFoundError(&APIError{StatusCode: http.StatusForbidden, Message: message}) {
			t.Errorf("want not found for 403 %q", message)
		}
	}
	if IsNotFoundError(&APIError{StatusCode: http.StatusForbidden, Message: "permission denied"}) {
		t.Error("an unrelated 403 must not be not found")
	}
}
