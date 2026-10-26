// Package offline reproduces exporter/provider integration defects without a
// Braintrust credential. It builds both binaries, points them at a local
// fixture API, and checks the exporter's output with Terraform through a
// development override. Requires terraform on PATH; skipped with -short.
package offline

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	org           = "00000000-0000-4000-8000-000000000001"
	otherOrg      = "90000000-0000-4000-8000-000000000001"
	project       = "00000000-0000-4000-8000-000000000002"
	projectGroup  = "00000000-0000-4000-8000-000000000022"
	secondProject = "00000000-0000-4000-8000-000000000023"
	role          = "00000000-0000-4000-8000-000000000003"
	user          = "00000000-0000-4000-8000-000000000004"
	builtinGroup  = "41000000-0000-4000-8000-000000000041"
	engineers     = "42000000-0000-4000-8000-000000000042"
	customGroup   = "43000000-0000-4000-8000-000000000043"
	serviceUser   = "44000000-0000-4000-8000-000000000044"
	staleACL      = "50000000-0000-4000-8000-000000000014"

	providerConfig = "terraform {\n required_providers {\n  braintrust = { source = \"braintrustdata/braintrust\" }\n }\n}\nprovider \"braintrust\" {}\n"
	commandTimeout = 60 * time.Second
)

type obj = map[string]any

// fixtures maps list endpoints to the objects they return.
type fixtures map[string][]obj

func baseFixtures() fixtures {
	return fixtures{
		"/v1/organization":  {{"id": org}},
		"/v1/project":       {{"id": project, "org_id": org, "name": "audit-project"}},
		"/v1/project_group": {},
		"/v1/role":          {{"id": role, "name": "Viewer", "org_id": nil}},
		"/v1/user":          {{"id": user, "email": "fixture@example.invalid"}},
		"/v1/group":         {},
		"/v1/acl":           {{"id": "00000000-0000-4000-8000-000000000005", "object_type": "project", "object_id": project, "user_id": user, "role_id": role, "_object_org_id": org}},
		"/v1/acl/list_org": {
			{"id": "00000000-0000-4000-8000-000000000005", "object_type": "project", "object_id": project, "user_id": user, "role_id": role, "_object_org_id": org},
			{"id": "30000000-0000-4000-8000-000000000012", "object_type": "prompt", "object_id": "40000000-0000-4000-8000-000000000013", "user_id": user, "permission": "read", "_object_org_id": org},
		},
		"/v1/dataset":       {{"id": "00000000-0000-4000-8000-000000000006", "project_id": project, "name": "audit-dataset", "tags": []string{"reviewed", "gold"}, "metadata": obj{"owner": "qa", "nested": obj{"tier": 1}}}},
		"/v1/project_score": {{"id": "90000000-0000-4000-8000-000000000006", "project_id": project, "name": "Audit Quality", "score_type": "categorical", "categories": []obj{{"name": "Low", "value": 0}, {"name": "High", "value": 1}}, "config": obj{"object_types": []string{"project_logs"}, "multi_select": false}}},
		"/v1/function": {
			{"id": "00000000-0000-4000-8000-000000000016", "project_id": project, "name": "Audit Prompt", "slug": "audit-prompt", "tags": []string{"audit"}, "function_data": obj{"type": "prompt"}, "prompt_data": obj{"prompt": "hello"}},
			{"id": "00000000-0000-4000-8000-000000000066", "project_id": project, "name": "Audit Scorer", "slug": "audit-scorer", "function_type": "scorer", "function_data": obj{"type": "code", "data": obj{"type": "inline", "runtime_context": obj{"runtime": "quickjs", "version": "ES2023"}, "code": "function main() { return 1; }"}}},
			{"id": "00000000-0000-4000-8000-000000000067", "project_id": project, "name": "Audit Preprocessor", "slug": "audit-preprocessor", "function_type": "preprocessor", "function_data": obj{"type": "global", "name": "thread", "function_type": "preprocessor"}},
		},
		"/v1/project_automation": {{"id": "00000000-0000-4000-8000-000000000007", "project_id": project, "name": "audit-retention", "config": obj{"event_type": "retention", "object_type": "project_logs", "retention_days": 90}}},
		"/v1/org_automation": {
			{"id": "00000000-0000-4000-8000-000000000051", "org_id": org, "name": "org-retention", "config": obj{"event_type": "retention", "object_type": "dataset", "retention_days": 0}},
			{"id": "00000000-0000-4000-8000-000000000052", "org_id": otherOrg, "name": "other-org-retention", "config": obj{"event_type": "retention", "object_type": "dataset", "retention_days": 30}},
		},
		"/v1/mcp_server": {
			{"id": "00000000-0000-4000-8000-000000000053", "project_id": project, "name": "audit-mcp", "url": "https://mcp.example.invalid", "deleted_at": nil},
			{"id": "00000000-0000-4000-8000-000000000054", "project_id": secondProject, "name": "unselected-mcp", "url": "https://other.example.invalid"},
			{"id": "00000000-0000-4000-8000-000000000055", "project_id": project, "name": "deleted-mcp", "url": "https://deleted.example.invalid", "deleted_at": "2026-01-01T00:00:00Z"},
		},
		"/v1/service_token": {{"id": "00000000-0000-4000-8000-000000000008", "name": "audit-token", "service_account_id": user, "org_id": org}},
		"/v1/ai_secret":     {{"id": "00000000-0000-4000-8000-000000000010", "name": "OpenAI", "type": "openai", "org_id": org}},
		"/v1/env_var":       {{"id": "00000000-0000-4000-8000-000000000011", "name": "ORDINARY_VALUE", "object_type": "project", "object_id": project}},
		"/v1/view": {
			{"id": "00000000-0000-4000-8000-000000000031", "object_type": "project", "object_id": project, "view_type": "logs", "name": "Errors", "view_data": obj{"search": obj{"filter": []any{}}}},
			{"id": "00000000-0000-4000-8000-000000000032", "object_type": "org_project", "object_id": org, "view_type": "monitor", "name": "Health", "options": obj{"viewType": "monitor", "options": obj{"projectId": project, "rangeValue": "7d"}}, "view_data": obj{"custom_charts": obj{"version": "0.0.0", "layout": obj{"type": "linear", "order": []string{"note"}}, "charts": obj{"note": obj{"type": "note", "markdown": "# Health"}}}}},
		},
	}
}

// fakeAPI serves fixtures and records every request URI.
type fakeAPI struct {
	mu         sync.Mutex
	fixtures   fixtures
	denyACLs   bool
	staleACLID string
	requests   []string
}

func (a *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	status, body := a.handle(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (a *fakeAPI) handle(r *http.Request) (int, any) {
	path := r.URL.Path
	if r.Method == http.MethodPatch {
		return a.patchGroup(r)
	}
	a.requests = append(a.requests, r.URL.RequestURI())
	switch {
	case a.denyACLs && path == "/v1/acl/list_org":
		return http.StatusForbidden, obj{"error": "fixture: missing read_acls permission"}
	case strings.HasPrefix(path, "/v1/acl/") && path != "/v1/acl/list_org":
		if lastSegment(path) == a.staleACLID {
			return http.StatusBadRequest, obj{"error": "ACL does not exist"}
		}
		return a.get("/v1/acl/list_org", path, "ACL does not exist")
	case strings.HasPrefix(path, "/v1/project_group/"):
		return a.get("/v1/project_group", path, "Project group does not exist")
	case strings.HasPrefix(path, "/v1/project_automation/"):
		return a.get("/v1/project_automation", path, "Project automation does not exist")
	case strings.HasPrefix(path, "/v1/group/"):
		return a.get("/v1/group", path, "Group does not exist")
	}
	objects := a.fixtures[path]
	if path == "/v1/view" {
		query := r.URL.Query()
		var views []obj
		for _, v := range objects {
			if v["object_type"] == query.Get("object_type") && v["object_id"] == query.Get("object_id") {
				views = append(views, v)
			}
		}
		objects = views
	}
	if strings.Contains(r.URL.RawQuery, "starting_after=") || objects == nil {
		objects = []obj{}
	}
	return http.StatusOK, obj{"objects": objects}
}

// get returns the object in list whose id is the last segment of path.
func (a *fakeAPI) get(list, path, notFound string) (int, any) {
	if found := a.find(list, lastSegment(path)); found != nil {
		return http.StatusOK, found
	}
	return http.StatusNotFound, obj{"error": notFound}
}

func (a *fakeAPI) find(list, id string) obj {
	for _, o := range a.fixtures[list] {
		if o["id"] == id {
			return o
		}
	}
	return nil
}

func (a *fakeAPI) patchGroup(r *http.Request) (int, any) {
	var group obj
	if strings.HasPrefix(r.URL.Path, "/v1/group/") {
		group = a.find("/v1/group", lastSegment(r.URL.Path))
	}
	if group == nil {
		return http.StatusNotFound, obj{"error": "Group does not exist"}
	}
	var patch struct {
		Add    []string `json:"add_member_users"`
		Remove []string `json:"remove_member_users"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		return http.StatusBadRequest, obj{"error": err.Error()}
	}
	members := map[string]bool{}
	for _, id := range a.members(group) {
		members[id] = true
	}
	for _, id := range patch.Add {
		members[id] = true
	}
	for _, id := range patch.Remove {
		delete(members, id)
	}
	sorted := make([]string, 0, len(members))
	for id := range members {
		sorted = append(sorted, id)
	}
	slices.Sort(sorted)
	group["member_users"] = sorted
	return http.StatusOK, group
}

func (a *fakeAPI) members(group obj) []string {
	members, _ := group["member_users"].([]string)
	return members
}

func lastSegment(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}

// Paths to the binaries under test, built once in TestMain.
var exporterPath, binDir string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	flag.Parse()
	if !testing.Short() {
		var err error
		binDir, err = os.MkdirTemp("", "bt-terraform-offline-bin-")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer os.RemoveAll(binDir)
		exporterPath = filepath.Join(binDir, "braintrust-terraforming")
		builds := map[string]string{
			filepath.Join(binDir, "terraform-provider-braintrust"): "github.com/braintrustdata/terraform-provider-braintrust",
			exporterPath: "github.com/braintrustdata/terraform-provider-braintrust/cmd/braintrust-terraforming",
		}
		for output, pkg := range builds {
			if out, err := exec.Command("go", "build", "-o", output, pkg).CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "build %s: %v\n%s", pkg, err, out)
				return 1
			}
		}
		if _, err := exec.LookPath("terraform"); err != nil {
			fmt.Fprintln(os.Stderr, "terraform must be on PATH for offline checks (see mise.toml), or run with -short")
			return 1
		}
	}
	return m.Run()
}

// harness runs the binaries against a fake API from a scratch workspace.
type harness struct {
	t         *testing.T
	api       *fakeAPI
	workspace string
	env       []string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	if testing.Short() {
		t.Skip("offline checks build binaries and run terraform")
	}
	api := &fakeAPI{fixtures: baseFixtures()}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	root := t.TempDir()
	config := filepath.Join(root, "terraform.rc")
	rc := fmt.Sprintf("provider_installation {\n  dev_overrides {\n    \"braintrustdata/braintrust\" = %q\n  }\n  direct {}\n}\n", binDir)
	if err := os.WriteFile(config, []byte(rc), 0o600); err != nil {
		t.Fatal(err)
	}
	// Drop ambient Braintrust and Terraform settings so only the fixture API is used.
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "BRAINTRUST_") || name == "TF_ACC" || name == "TF_LOG" || name == "TF_LOG_PATH" || name == "TF_CLI_CONFIG_FILE" {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"BRAINTRUST_API_KEY=offline-fixture-token",
		"BRAINTRUST_API_URL="+server.URL,
		"TF_IN_AUTOMATION=1",
		"CHECKPOINT_DISABLE=1",
		"TF_CLI_CONFIG_FILE="+config,
	)
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, api: api, workspace: workspace, env: env}
}

func (h *harness) writeFile(name, content string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.workspace, name), []byte(content), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

// run executes a command in the workspace and returns its output and exit code.
// Timeouts and launch failures fail the test rather than count as application exits.
func (h *harness) run(name string, args ...string) (stdout, stderr string, exitCode int) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = h.workspace
	cmd.Env = h.env
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if ctx.Err() != nil {
		h.t.Fatalf("run %s: %v", name, ctx.Err())
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return out.String(), errOut.String(), exitErr.ExitCode()
	} else if err != nil {
		h.t.Fatalf("run %s: %v", name, err)
	}
	return out.String(), errOut.String(), 0
}

func (h *harness) requests() []string {
	h.api.mu.Lock()
	defer h.api.mu.Unlock()
	return append([]string(nil), h.api.requests...)
}

func (h *harness) groupMembers() []string {
	h.api.mu.Lock()
	defer h.api.mu.Unlock()
	return h.api.members(h.api.fixtures["/v1/group"][0])
}

// result is one exporter run plus the Terraform check of its output.
type result struct {
	t        *testing.T
	scenario *scenario
	hcl      string
	stderr   string
	exitCode int
	tfStdout string
	requests []string
}

func (r *result) has(s string) bool           { return strings.Contains(r.hcl, s) }
func (r *result) count(s string) int          { return strings.Count(r.hcl, s) }
func (r *result) matches(pattern string) bool { return regexp.MustCompile(pattern).MatchString(r.hcl) }
func (r *result) reported(s string) bool      { return strings.Contains(r.stderr, s) }
func (r *result) failIf(bad bool, msg string) { r.t.Helper(); r.failIfNot(!bad, msg) }
func (r *result) failIfNot(ok bool, msg string) {
	r.t.Helper()
	if !ok {
		r.t.Error(msg)
	}
}

// requestsTo counts requests whose path is exactly path.
func (r *result) requestsTo(path string) int {
	n := 0
	for _, request := range r.requests {
		if requestPath(request) == path {
			n++
		}
	}
	return n
}

// aclReads counts individual ACL reads (not list_org discovery).
func (r *result) aclReads() int {
	n := 0
	for _, request := range r.requests {
		path := requestPath(request)
		if strings.HasPrefix(path, "/v1/acl/") && path != "/v1/acl/list_org" {
			n++
		}
	}
	return n
}

func requestPath(requestURI string) string {
	path, _, _ := strings.Cut(requestURI, "?")
	return path
}

type scenario struct {
	name    string
	command string // generate or genimport
	types   string
	flags   []string
	plan    bool // run terraform plan instead of validate
	// unsupported discovery must fail without emitting a partial export.
	wantFail bool
	setup    func(*fakeAPI)
	check    func(*result)
}

func TestOfflineScenarios(t *testing.T) {
	for i := range scenarios {
		sc := &scenarios[i]
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			if sc.setup != nil {
				sc.setup(h.api)
			}
			args := append([]string{sc.command, "--resource-types", sc.types}, sc.flags...)
			hcl, stderr, exitCode := h.run(exporterPath, args...)
			r := &result{t: t, scenario: sc, hcl: hcl, stderr: stderr, exitCode: exitCode, requests: h.requests()}
			t.Cleanup(func() {
				if t.Failed() {
					t.Logf("exporter exit %d\nstderr:\n%s\nstdout:\n%s\nterraform:\n%s", exitCode, stderr, hcl, r.tfStdout)
				}
			})

			if strings.Contains(sc.types, "braintrust_acl") {
				r.failIf(r.aclReads() > 0 && slices.Contains(sc.flags, "--verify-acl-targets=false"), fmt.Sprintf("list-only ACL discovery made %d individual reads", r.aclReads()))
				r.failIf(r.requestsTo("/v1/acl") > 0, "ACL discovery made a target-scoped request")
			}
			if sc.wantFail {
				r.failIf(exitCode == 0 || strings.TrimSpace(hcl) != "", "unsupported discovery must fail without emitting a partial export")
				return
			}

			h.writeFile("generated.tf", hcl)
			h.writeFile("provider.tf", providerConfig)
			tfArgs := []string{"validate", "-json", "-no-color"}
			if sc.plan {
				tfArgs = []string{"plan", "-input=false", "-refresh=false", "-no-color"}
			}
			tfStdout, tfStderr, tfExit := h.run("terraform", tfArgs...)
			r.tfStdout = tfStdout + tfStderr
			if exitCode != 0 || tfExit != 0 {
				t.Fatalf("expected valid generated Terraform (exporter exit %d, terraform exit %d)", exitCode, tfExit)
			}
			if sc.check != nil {
				sc.check(r)
			}
		})
	}
}

// Adding a human to a built-in group must not disturb its service members.
func TestGroupMembershipLifecycle(t *testing.T) {
	h := newHarness(t)
	h.api.fixtures["/v1/group"] = []obj{{"id": builtinGroup, "org_id": org, "name": "All AI Provider Access", "member_users": []string{serviceUser}}}
	h.writeFile("main.tf", providerConfig+fmt.Sprintf("resource \"braintrust_group_membership\" \"human\" {\n  group_id = %q\n  user_id = %q\n}\n", builtinGroup, user))

	stdout, stderr, exitCode := h.run("terraform", "apply", "-auto-approve", "-input=false", "-no-color")
	if exitCode != 0 {
		t.Fatalf("apply failed:\n%s%s", stdout, stderr)
	}
	// The fake API keeps members sorted.
	if got := h.groupMembers(); !slices.Equal(got, []string{user, serviceUser}) {
		t.Errorf("adding the human changed the service account membership: %v", got)
	}
	stdout, stderr, exitCode = h.run("terraform", "destroy", "-auto-approve", "-input=false", "-no-color")
	if exitCode != 0 {
		t.Fatalf("destroy failed:\n%s%s", stdout, stderr)
	}
	if got := h.groupMembers(); !slices.Equal(got, []string{serviceUser}) {
		t.Errorf("destroy removed another member or left the human member: %v", got)
	}
}
