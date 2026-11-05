package compat

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
)

const (
	projectID = "00000000-0000-4000-8000-000000000002"
	orgID     = "00000000-0000-4000-8000-000000000001"
)

// projectAPI implements only the project lifecycle exercised by this suite.
// Unexpected routes fail rather than returning a permissive fixture response.
type projectAPI struct {
	mu      sync.Mutex
	project map[string]any
}

func (a *projectAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	var body any
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/project":
		if a.project != nil {
			http.Error(w, `{"error":"project already exists"}`, http.StatusConflict)
			return
		}
		var fields map[string]any
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.project = map[string]any{
			"id": projectID, "org_id": orgID,
			"name": fields["name"], "description": fields["description"],
		}
		body = a.project
	case r.URL.Path == "/v1/project/"+projectID:
		if a.project == nil {
			http.Error(w, `{"error":"project does not exist"}`, http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			body = a.project
		case http.MethodPatch:
			var patch map[string]any
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			for key, value := range patch {
				a.project[key] = value
			}
			body = a.project
		case http.MethodDelete:
			body = a.project
			a.project = nil
		default:
			http.Error(w, `{"error":"unexpected method"}`, http.StatusMethodNotAllowed)
			return
		}
	default:
		http.Error(w, `{"error":"unexpected route"}`, http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func (a *projectAPI) assertProject(t *testing.T, description string, destroyed bool) {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	if destroyed {
		if a.project != nil {
			t.Fatalf("project remains in API after destroy: %v", a.project)
		}
		return
	}
	for key, want := range map[string]any{"id": projectID, "org_id": orgID, "name": "engine-compat", "description": description} {
		if got := a.project[key]; got != want {
			t.Errorf("API project %s = %v, want %v", key, got, want)
		}
	}
}
