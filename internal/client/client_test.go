package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteDoesNotRetryAmbiguousServerError(t *testing.T) {
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
	err = c.doRequest(context.Background(), http.MethodPost, "/v1/project", map[string]string{"name": "fixture"}, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError || requests != 1 {
		t.Fatalf("want one ambiguous write and HTTP 500; got requests=%d error=%v", requests, err)
	}
	if !strings.Contains(err.Error(), "outcome is unknown") {
		t.Fatalf("write error must explain uncertainty: %v", err)
	}
	if !IsUncertainWriteError(err) {
		t.Fatalf("write error must be classifiable: %v", err)
	}
}

func TestWriteDoesNotRetryDroppedResponse(t *testing.T) {
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
	err = c.doRequest(context.Background(), http.MethodPost, "/v1/project", map[string]string{"name": "fixture"}, nil)
	if err == nil || !strings.Contains(err.Error(), "outcome is unknown") || requests != 1 {
		t.Fatalf("want one ambiguous write; got requests=%d error=%v", requests, err)
	}
	if !IsUncertainWriteError(err) {
		t.Fatalf("write error must be classifiable: %v", err)
	}
}

func TestReadRetriesTransientServerError(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"project-1"}`))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ID string `json:"id"`
	}
	err = c.doRequest(context.Background(), http.MethodGet, "/v1/project/project-1", nil, &result)
	if err != nil || requests != 2 || result.ID != "project-1" {
		t.Fatalf("want successful second read; got requests=%d result=%q error=%v", requests, result.ID, err)
	}
}

func TestCreateDoesNotAdoptExistingRemoteObject(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-bt-found-existing", "true")
		_, _ = w.Write([]byte(`{"id":"existing-id","name":"existing"}`))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateProject(context.Background(), CreateProjectRequest{Name: "existing"}); err == nil || !strings.Contains(err.Error(), "import") {
		t.Fatalf("creating an existing project must require import; got %v", err)
	}
	if _, err := c.CreateProjectAutomation(context.Background(), CreateProjectAutomationRequest{ProjectID: "project", Name: "existing"}); err == nil || !strings.Contains(err.Error(), "import") {
		t.Fatalf("creating an existing automation must require import; got %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected one request per create, got %d", requests)
	}
}
