// Package compat checks the same provider behavior under Terraform and OpenTofu.
// Keep this suite small; add scenarios for engine compatibility regressions,
// not comprehensive resource coverage. Requires both CLIs; skipped with -short.
package compat

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/tests/internal/testcli"
)

type engine struct {
	name   string
	binary string
}

var engines = []engine{
	{name: "terraform", binary: "terraform"},
	{name: "opentofu", binary: "tofu"},
}

func TestResourceLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("engine compatibility builds the provider and runs Terraform and OpenTofu")
	}
	binDir := t.TempDir()
	binary := "terraform-provider-braintrust"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if out, err := exec.Command("go", "build", "-mod=readonly", "-o", filepath.Join(binDir, binary), "github.com/braintrustdata/terraform-provider-braintrust").CombinedOutput(); err != nil {
		t.Fatalf("build provider: %v\n%s", err, out)
	}

	for _, engine := range engines {
		t.Run(engine.name, func(t *testing.T) {
			t.Parallel()
			if _, err := exec.LookPath(engine.binary); err != nil {
				t.Fatalf("%s must be on PATH (run mise install and mise exec -- make test-compat): %v", engine.binary, err)
			}
			api := &projectAPI{}
			server := httptest.NewServer(api)
			t.Cleanup(server.Close)
			workspace, env := testcli.Workspace(t, binDir, server.URL)
			run := func(wantExit int, args ...string) string {
				t.Helper()
				stdout, stderr, exit := testcli.Run(t, workspace, env, engine.binary, args...)
				if exit != wantExit {
					t.Fatalf("%s %v: exit %d, want %d\n%s%s", engine.binary, args, exit, wantExit, stdout, stderr)
				}
				return stdout
			}
			t.Log(strings.TrimSpace(run(0, "version")))
			writeConfig := func(description string) {
				t.Helper()
				config := fmt.Sprintf(`terraform {
  required_providers {
    braintrust = { source = "braintrustdata/braintrust" }
  }
}
provider "braintrust" {}
resource "braintrust_project" "smoke" {
  name        = "engine-compat"
  description = %q
}
`, description)
				if err := os.WriteFile(filepath.Join(workspace, "main.tf"), []byte(config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			assertState := func(description string, destroyed bool) {
				t.Helper()
				var state struct {
					Values struct {
						RootModule struct {
							Resources []struct {
								Address string         `json:"address"`
								Values  map[string]any `json:"values"`
							} `json:"resources"`
						} `json:"root_module"`
					} `json:"values"`
				}
				if err := json.Unmarshal([]byte(run(0, "show", "-json")), &state); err != nil {
					t.Fatal(err)
				}
				resources := state.Values.RootModule.Resources
				if destroyed {
					if len(resources) != 0 {
						t.Fatalf("resources remain after destroy: %+v", resources)
					}
					return
				}
				if len(resources) != 1 || resources[0].Address != "braintrust_project.smoke" {
					t.Fatalf("expected only braintrust_project.smoke in state: %+v", resources)
				}
				for key, want := range map[string]any{"id": projectID, "org_id": orgID, "name": "engine-compat", "description": description} {
					if got := resources[0].Values[key]; got != want {
						t.Errorf("state %s = %v, want %v", key, got, want)
					}
				}
			}
			applyPlan := func(action string, destroy bool) {
				t.Helper()
				args := []string{"plan", "-input=false", "-no-color", "-detailed-exitcode", "-out=change.plan"}
				if destroy {
					args = append(args, "-destroy")
				}
				run(2, args...)
				var plan struct {
					ResourceChanges []struct {
						Address string `json:"address"`
						Change  struct {
							Actions []string `json:"actions"`
						} `json:"change"`
					} `json:"resource_changes"`
				}
				if err := json.Unmarshal([]byte(run(0, "show", "-json", "change.plan")), &plan); err != nil {
					t.Fatal(err)
				}
				if len(plan.ResourceChanges) != 1 || plan.ResourceChanges[0].Address != "braintrust_project.smoke" || !reflect.DeepEqual(plan.ResourceChanges[0].Change.Actions, []string{action}) {
					t.Fatalf("expected only a project %s, got %+v", action, plan.ResourceChanges)
				}
				run(0, "apply", "-input=false", "-no-color", "change.plan")
			}

			writeConfig("created by compatibility test")
			run(0, "validate", "-no-color")
			applyPlan("create", false)
			assertState("created by compatibility test", false)
			api.assertProject(t, "created by compatibility test", false)
			run(0, "plan", "-input=false", "-no-color", "-detailed-exitcode")

			writeConfig("updated by compatibility test")
			applyPlan("update", false)
			assertState("updated by compatibility test", false)
			api.assertProject(t, "updated by compatibility test", false)
			run(0, "plan", "-input=false", "-no-color", "-detailed-exitcode")

			applyPlan("delete", true)
			assertState("", true)
			api.assertProject(t, "", true)
		})
	}
}
