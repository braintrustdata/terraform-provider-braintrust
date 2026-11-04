package offline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serviceToken keeps creation-only keys separate from the list/GET fixtures.
// Called with fakeAPI.mu held, like the other fixture handlers.
func (a *fakeAPI) serviceToken(r *http.Request) (int, any) {
	const endpoint = "/v1/service_token"
	switch r.Method {
	case http.MethodPost:
		if r.URL.Path != endpoint {
			return http.StatusNotFound, obj{"error": "Unknown token endpoint"}
		}
		var input obj
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return http.StatusBadRequest, obj{"error": "Invalid token request"}
		}
		n := len(a.tokenCreations) + 1
		metadata := obj{
			"id":   fmt.Sprintf("80000000-0000-4000-8000-%012d", n),
			"name": input["name"], "service_account_id": input["service_account_id"],
			"org_id": org, "preview_name": "offline-token", "created": "2026-01-01T00:00:00Z",
		}
		a.fixtures[endpoint] = append(a.fixtures[endpoint], metadata)
		created := make(obj, len(metadata)+2)
		for k, v := range metadata {
			created[k] = v
		}
		created["key"] = fmt.Sprintf("sk-offline-ephemeral-secret-%012d", n)
		created["expires_in_seconds"] = input["expires_in_seconds"]
		a.tokenCreations = append(a.tokenCreations, created)
		return http.StatusOK, created
	case http.MethodDelete:
		if a.tokenDeleteStatus != 0 {
			return a.tokenDeleteStatus, obj{"error": "fixture: token revocation denied"}
		}
		id := lastSegment(r.URL.Path)
		for i, token := range a.fixtures[endpoint] {
			if token["id"] == id {
				a.tokenDeletes = append(a.tokenDeletes, id)
				a.fixtures[endpoint] = append(a.fixtures[endpoint][:i], a.fixtures[endpoint][i+1:]...)
				return http.StatusOK, token
			}
		}
		if a.tokenMissingStatus != 0 {
			return a.tokenMissingStatus, obj{"error": "Service token does not exist"}
		}
		return http.StatusNotFound, obj{"error": "Service token does not exist"}
	case http.MethodGet:
		if r.URL.Path != endpoint {
			return a.get(endpoint, r.URL.Path, "Service token does not exist")
		}
		objects := []obj{}
		if r.URL.Query().Get("starting_after") == "" {
			for _, token := range a.fixtures[endpoint] {
				if name := r.URL.Query().Get("name"); name == "" || token["name"] == name {
					objects = append(objects, token)
				}
			}
		}
		return http.StatusOK, obj{"objects": objects}
	default:
		return http.StatusMethodNotAllowed, obj{"error": "Unsupported token method"}
	}
}

func newEphemeralTokenHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	// Execute the published example, including its exact delivery script; do
	// not substitute embedded HCL that could drift from the consumer path.
	source := filepath.Join("..", "..", "examples", "ephemeral-service-token")
	files, err := filepath.Glob(filepath.Join(source, "*.tf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("ephemeral service-token example contains no Terraform files")
	}
	files = append(files, filepath.Join(source, "store.sh"))
	for _, path := range files {
		name := filepath.Base(path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		h.writeFile(name, string(data))
		if name == "store.sh" {
			if err := os.Chmod(filepath.Join(h.workspace, name), 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Never let ambient variables, CLI flags, or workspace settings alter this
	// example. Preserve only the harness's explicit Terraform settings.
	env := make([]string, 0, len(h.env)+4)
	for _, kv := range h.env {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "TF_") && name != "TF_CLI_CONFIG_FILE" && name != "TF_IN_AUTOMATION" {
			continue
		}
		env = append(env, kv)
		if name == "BRAINTRUST_API_URL" {
			_, value, _ := strings.Cut(kv, "=")
			env = append(env, "TF_VAR_api_url="+value)
		}
	}
	env = append(env, "TF_VAR_service_account_id="+serviceUser, "TF_VAR_token_name=offline-ephemeral-demo", "TF_VAR_expires_in_seconds=3600")
	h.env = env
	return h
}

func (h *harness) assertTokenCounts(created, deleted int) {
	h.t.Helper()
	h.api.mu.Lock()
	defer h.api.mu.Unlock()
	if len(h.api.tokenCreations) != created || len(h.api.tokenDeletes) != deleted {
		h.t.Fatalf("token lifecycle: got %d creates/%d deletes, want %d/%d", len(h.api.tokenCreations), len(h.api.tokenDeletes), created, deleted)
	}
	for _, token := range h.api.tokenCreations {
		deleted := false
		for _, id := range h.api.tokenDeletes {
			deleted = deleted || token["id"] == id
		}
		if (h.api.find("/v1/service_token", token["id"].(string)) == nil) != deleted {
			h.t.Fatal("remote token presence does not match revocation history")
		}
	}
}

func (h *harness) assertTokenArtifactsAbsent(outputs []string) {
	h.t.Helper()
	h.api.mu.Lock()
	var keys []string
	for _, token := range h.api.tokenCreations {
		keys = append(keys, token["key"].(string))
	}
	h.api.mu.Unlock()
	for _, output := range outputs {
		assertSecretAbsent(h.t, []byte(output), keys...)
	}
	for _, pattern := range []string{"*.tfstate", "*.tfstate.*", "*.tfplan", ".service-token-id"} {
		paths, err := filepath.Glob(filepath.Join(h.workspace, pattern))
		if err != nil {
			h.t.Fatal(err)
		}
		for _, path := range paths {
			if strings.HasSuffix(path, ".tfplan") {
				assertSecretPlanAbsent(h.t, path, keys...)
			} else {
				assertSecretFileAbsent(h.t, path, keys...)
			}
		}
	}
}

func (h *harness) assertStoredToken(index int) []byte {
	h.t.Helper()
	path := filepath.Join(h.workspace, ".secrets", "service-token")
	data, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatal(err)
	}
	h.api.mu.Lock()
	token := h.api.tokenCreations[index]
	want := token["id"].(string) + "\n" + token["key"].(string) + "\n"
	wantID := token["id"].(string) + "\n"
	valid := token["service_account_id"] == serviceUser && token["expires_in_seconds"] == float64(3600)
	h.api.mu.Unlock()
	if string(data) != want {
		h.t.Fatal("store does not contain the newly created token ID and raw key")
	}
	if !valid {
		h.t.Fatal("token creation did not preserve account or lifetime inputs")
	}
	receipt, err := os.ReadFile(filepath.Join(h.workspace, ".service-token-id"))
	if err != nil || string(receipt) != wantID {
		h.t.Fatal("non-secret recovery receipt does not contain exactly the token ID")
	}
	for _, target := range []string{filepath.Dir(path), path} {
		info, err := os.Stat(target)
		if err != nil {
			h.t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			h.t.Fatal("token store permissions allow group or other access")
		}
	}
	return data
}

func TestEphemeralServiceTokenExample(t *testing.T) {
	h := newEphemeralTokenHarness(t)
	var outputs []string
	run := func(args ...string) {
		t.Helper()
		stdout, stderr, code := h.run("terraform", args...)
		outputs = append(outputs, stdout, stderr)
		h.assertTokenArtifactsAbsent(outputs)
		if code != 0 {
			t.Fatalf("terraform %s failed:\n%s%s", args[0], stdout, stderr)
		}
	}
	plan := func(name string) {
		t.Helper()
		run("plan", "-input=false", "-no-color", "-out="+name)
	}
	apply := func(name string) {
		t.Helper()
		run("apply", "-input=false", "-no-color", name)
	}

	// Neither an ordinary fresh plan nor repeated saved plans may mint.
	run("plan", "-input=false", "-no-color")
	h.assertTokenCounts(0, 0)
	plan("initial.tfplan")
	h.assertTokenCounts(0, 0)
	plan("repeated.tfplan")
	h.assertTokenCounts(0, 0)
	apply("repeated.tfplan")
	h.assertTokenCounts(1, 0)
	original := h.assertStoredToken(0)
	if _, err := os.Stat(filepath.Join(h.workspace, "terraform.tfstate")); err != nil {
		t.Fatal(err)
	}

	// Repeat plans and applies must leave both token and store untouched.
	for _, name := range []string{"repeat.tfplan", "stable.tfplan"} {
		plan(name)
		h.assertTokenCounts(1, 0)
		apply(name)
		h.assertTokenCounts(1, 0)
		if !bytes.Equal(h.assertStoredToken(0), original) {
			t.Fatal("repeat apply changed stored credential bytes")
		}
	}

	// The non-secret receipt prevents reissuance if only the key is lost,
	// and still lets destroy revoke the token.
	store := filepath.Join(h.workspace, ".secrets", "service-token")
	if err := os.Remove(store); err != nil {
		t.Fatal(err)
	}
	plan("missing-store.tfplan")
	apply("missing-store.tfplan")
	h.assertTokenCounts(1, 0)
	run("destroy", "-input=false", "-no-color", "-auto-approve")
	h.assertTokenCounts(1, 1)
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Fatal("destroy did not remove the revoked token's store entry")
	}
	plan("recreate.tfplan")
	h.assertTokenCounts(1, 1)
	apply("recreate.tfplan")
	h.assertTokenCounts(2, 1)
	h.assertStoredToken(1)
	run("destroy", "-input=false", "-no-color", "-auto-approve")
	h.assertTokenCounts(2, 2)
}

func TestEphemeralServiceTokenStoreFailure(t *testing.T) {
	h := newEphemeralTokenHarness(t)
	// Inject a shell filesystem-operation failure without modifying the exact
	// example script or changing fileexists between saved plan and apply.
	// Unlike chmod denial, this also fails under privileged test runners.
	failingBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(failingBin, "mkdir"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for i, kv := range h.env {
		if strings.HasPrefix(kv, "PATH=") {
			h.env[i] = "PATH=" + failingBin + string(os.PathListSeparator) + strings.TrimPrefix(kv, "PATH=")
		}
	}
	stdout, stderr, code := h.run("terraform", "plan", "-input=false", "-no-color", "-out=failure.tfplan")
	outputs := []string{stdout, stderr}
	if code != 0 {
		t.Fatalf("failure-path plan failed:\n%s%s", stdout, stderr)
	}
	h.assertTokenCounts(0, 0)
	stdout, stderr, code = h.run("terraform", "apply", "-input=false", "-no-color", "failure.tfplan")
	outputs = append(outputs, stdout, stderr)
	h.assertTokenArtifactsAbsent(outputs)
	if code == 0 {
		t.Fatal("apply succeeded despite failed token delivery")
	}
	// Terraform skips destroy provisioners on tainted resources. The separate
	// owner is not tainted and can still revoke using the non-secret receipt.
	h.assertTokenCounts(1, 0)
	if _, err := os.Stat(filepath.Join(h.workspace, "terraform.tfstate")); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = h.run("terraform", "destroy", "-input=false", "-no-color", "-auto-approve")
	outputs = append(outputs, stdout, stderr)
	h.assertTokenArtifactsAbsent(outputs)
	if code != 0 {
		t.Fatalf("failure-path bookkeeping cleanup failed:\n%s%s", stdout, stderr)
	}
	h.assertTokenCounts(1, 1)
}

func TestEphemeralServiceTokenRevocationRetry(t *testing.T) {
	for _, missingStatus := range []int{0, http.StatusNotFound, http.StatusForbidden, http.StatusBadRequest} {
		t.Run(fmt.Sprintf("missing_status=%d", missingStatus), func(t *testing.T) {
			h := newEphemeralTokenHarness(t)
			run := func(action string, wantSuccess bool) {
				t.Helper()
				stdout, stderr, code := h.run("terraform", action, "-input=false", "-no-color", "-auto-approve")
				h.assertTokenArtifactsAbsent([]string{stdout, stderr})
				if (code == 0) != wantSuccess {
					t.Fatalf("unexpected %s result (%d):\n%s%s", action, code, stdout, stderr)
				}
			}
			run("apply", true)
			original := h.assertStoredToken(0)
			h.api.mu.Lock()
			h.api.tokenDeleteStatus = http.StatusForbidden
			h.api.mu.Unlock()
			run("destroy", false)
			h.assertTokenCounts(1, 0)
			if !bytes.Equal(original, h.assertStoredToken(0)) {
				t.Fatal("failed revocation changed the stored credential")
			}
			h.api.mu.Lock()
			h.api.tokenDeleteStatus = 0
			h.api.tokenMissingStatus = missingStatus
			if missingStatus != 0 {
				id := h.api.tokenCreations[0]["id"].(string)
				req, err := http.NewRequest(http.MethodDelete, "http://fixture/v1/service_token/"+id, nil)
				if err != nil {
					h.api.mu.Unlock()
					t.Fatal(err)
				}
				h.api.serviceToken(req)
			}
			h.api.mu.Unlock()
			run("destroy", true)
			h.assertTokenCounts(1, 1)
			for _, name := range []string{".secrets/service-token", ".service-token-id"} {
				if _, err := os.Stat(filepath.Join(h.workspace, name)); !os.IsNotExist(err) {
					t.Fatalf("successful cleanup left %s behind", name)
				}
			}
		})
	}
}

func TestEphemeralServiceTokenAdoptsPreviousDemoStore(t *testing.T) {
	h := newEphemeralTokenHarness(t)
	run := func(args ...string) {
		t.Helper()
		stdout, stderr, code := h.run("terraform", args...)
		h.assertTokenArtifactsAbsent([]string{stdout, stderr})
		if code != 0 {
			t.Fatalf("terraform %s failed:\n%s%s", args[0], stdout, stderr)
		}
	}
	run("apply", "-input=false", "-no-color", "-auto-approve")
	original := h.assertStoredToken(0)
	// The previous demo left only the store, with no owner in state or receipt.
	run("state", "rm", "terraform_data.delivery", "terraform_data.token")
	if err := os.Remove(filepath.Join(h.workspace, ".service-token-id")); err != nil {
		t.Fatal(err)
	}
	run("apply", "-input=false", "-no-color", "-auto-approve")
	h.assertTokenCounts(1, 0)
	if !bytes.Equal(original, h.assertStoredToken(0)) {
		t.Fatal("adopting the previous demo replaced its credential")
	}
	run("destroy", "-input=false", "-no-color", "-auto-approve")
	h.assertTokenCounts(1, 1)
	if _, err := os.Stat(filepath.Join(h.workspace, ".secrets", "service-token")); !os.IsNotExist(err) {
		t.Fatal("destroy did not remove the adopted store entry")
	}
}
