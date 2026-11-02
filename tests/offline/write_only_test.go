package offline

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// Values live only in the fake service, never in its API responses.
func (a *fakeAPI) writeSecret(r *http.Request) (int, any) {
	if a.failSecretWrites {
		return http.StatusForbidden, obj{"error": "fixture: secret writes denied"}
	}
	list := r.URL.Path
	if r.Method == http.MethodPatch {
		list = list[:strings.LastIndex(list, "/")]
	}
	var input obj
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		return http.StatusBadRequest, obj{"error": err.Error()}
	}
	objects := a.fixtures[list]
	if len(objects) != 1 {
		return http.StatusNotFound, obj{"error": "fixture requires one secret"}
	}
	object := objects[0]
	if r.Method == http.MethodPatch && lastSegment(r.URL.Path) != object["id"] {
		return http.StatusNotFound, obj{"error": "Secret does not exist"}
	}
	field := "secret"
	if list == "/v1/env_var" {
		field = "value"
	}
	for key, value := range input {
		if key == field {
			if a.secretValues == nil {
				a.secretValues = make(map[string]any)
			}
			a.secretValues[list] = value
			a.secretWrites++
		} else {
			object[key] = value
		}
	}
	return http.StatusOK, object
}

func TestWriteOnlySecrets(t *testing.T) {
	for _, kind := range []string{"ai_secret", "env_var"} {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t)
			field, scope := "secret", "type = \"openai\""
			if kind == "env_var" {
				field, scope = "value", fmt.Sprintf("object_type = \"project\"\nobject_id = %q", project)
			}
			endpoint := "/v1/" + kind
			id := h.api.fixtures[endpoint][0]["id"].(string)
			const planned = "fixture-secret-for-plan"
			const initial = "fixture-secret-for-apply"
			const rotated = "fixture-secret-for-rotation"
			const legacy = "fixture-secret-from-legacy-state"
			configure := func(name string, version int, supplied bool) {
				valueConfig := ""
				if supplied {
					valueConfig = fmt.Sprintf("%s = var.payload\n", field)
				}
				if version > 0 {
					valueConfig += fmt.Sprintf("%s_version = %d\n", field, version)
				}
				h.writeFile("main.tf", providerConfig+fmt.Sprintf(`
variable "payload" {
  type = string
  sensitive = true
  ephemeral = true
}
resource "braintrust_%s" "test" {
  name = %q
  %s
  %s
}
`, kind, name, scope, valueConfig))
			}
			setPayload := func(value string) {
				for i, kv := range h.env {
					if strings.HasPrefix(kv, "TF_VAR_payload=") {
						h.env[i] = "TF_VAR_payload=" + value
						return
					}
				}
				h.env = append(h.env, "TF_VAR_payload="+value)
			}
			apply := func(args ...string) {
				t.Helper()
				stdout, stderr, code := h.run("terraform", append([]string{"apply", "-input=false", "-no-color", "-auto-approve"}, args...)...)
				if code != 0 {
					t.Fatalf("apply failed:\n%s%s", stdout, stderr)
				}
				assertSecretAbsent(t, []byte(stdout+stderr), planned, initial, rotated)
				assertSecretFileAbsent(t, filepath.Join(h.workspace, "terraform.tfstate"), planned, initial, rotated, legacy)
			}
			assertRemote := func(value any, writes int) {
				t.Helper()
				h.api.mu.Lock()
				defer h.api.mu.Unlock()
				if h.api.secretValues[endpoint] != value || h.api.secretWrites != writes {
					t.Fatalf("unexpected remote secret or write count: got %d writes, want %d", h.api.secretWrites, writes)
				}
			}
			plan := func(want int) {
				t.Helper()
				stdout, stderr, code := h.run("terraform", "plan", "-input=false", "-no-color", "-detailed-exitcode", "-out=secret.tfplan")
				if code != want {
					t.Fatalf("plan exit %d, want %d:\n%s%s", code, want, stdout, stderr)
				}
				assertSecretAbsent(t, []byte(stdout+stderr), planned, initial, rotated, legacy)
				assertSecretPlanAbsent(t, filepath.Join(h.workspace, "secret.tfplan"), planned, initial, rotated, legacy)
			}

			// A saved plan must not capture the secret, and apply must use the
			// newly supplied ephemeral value rather than the value used at plan.
			configure("write-only", 1, true)
			setPayload(planned)
			plan(2)
			setPayload(initial)
			apply("secret.tfplan")
			assertRemote(initial, 1)

			// Changing only the secret doesn't rotate; changing other metadata
			// must not accidentally resend it either.
			setPayload(rotated)
			plan(0)
			configure("renamed", 1, true)
			apply()
			assertRemote(initial, 1)
			configure("renamed", 2, true)
			apply()
			assertRemote(rotated, 2)

			// A failed rotation must not commit its version. The same config
			// must still plan a change and successfully retry afterward.
			configure("renamed", 3, true)
			setPayload(initial)
			h.api.mu.Lock()
			h.api.failSecretWrites = true
			h.api.mu.Unlock()
			stdout, stderr, code := h.run("terraform", "apply", "-input=false", "-no-color", "-auto-approve")
			if code == 0 {
				t.Fatal("denied rotation succeeded")
			}
			assertSecretAbsent(t, []byte(stdout+stderr), planned, initial, rotated)
			assertSecretFileAbsent(t, filepath.Join(h.workspace, "terraform.tfstate"), planned, initial, rotated)
			assertRemote(rotated, 2)
			h.api.mu.Lock()
			h.api.failSecretWrites = false
			h.api.mu.Unlock()
			plan(2)
			apply("secret.tfplan")
			assertRemote(initial, 3)

			// Removing value management is not a request to erase a secret.
			configure("unmanaged-value", 0, false)
			apply()
			assertRemote(initial, 3)
			if kind == "env_var" {
				configure("unmanaged-value", 4, false)
				apply()
				assertRemote(nil, 4)
			}
			assertSecretFileAbsent(t, filepath.Join(h.workspace, "terraform.tfstate.backup"), planned, initial, rotated)

			// Simulate a v0 state snapshot. Upgrading must erase the plaintext,
			// preserve the ID, and leave the remote value alone.
			configure("unmanaged-value", 0, false)
			statePath := filepath.Join(h.workspace, "terraform.tfstate")
			stateBytes, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			var state obj
			if err := json.Unmarshal(stateBytes, &state); err != nil {
				t.Fatal(err)
			}
			instance := state["resources"].([]any)[0].(map[string]any)["instances"].([]any)[0].(map[string]any)
			instance["schema_version"] = 0
			attrs := instance["attributes"].(map[string]any)
			attrs[field] = legacy
			delete(attrs, field+"_version")
			stateBytes, err = json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			h.writeFile("terraform.tfstate", string(stateBytes))
			plan(0)
			apply("secret.tfplan")
			if kind == "env_var" {
				assertRemote(nil, 4)
			} else {
				assertRemote(initial, 3)
			}
			stateBytes, err = os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(stateBytes, &state); err != nil {
				t.Fatal(err)
			}
			instance = state["resources"].([]any)[0].(map[string]any)["instances"].([]any)[0].(map[string]any)
			attrs = instance["attributes"].(map[string]any)
			if attrs["id"] != id || attrs[field] != nil || instance["schema_version"] != float64(1) {
				t.Fatal("state upgrade lost identity, retained the secret, or did not advance schema version")
			}
		})
	}
}

func assertSecretAbsent(t *testing.T, data []byte, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(string(data), secret) {
			t.Fatal("secret persisted in Terraform artifact or output")
		}
	}
}

func assertSecretFileAbsent(t *testing.T, filename string, secrets ...string) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	assertSecretAbsent(t, data, secrets...)
}

func assertSecretPlanAbsent(t *testing.T, filename string, secrets ...string) {
	t.Helper()
	plan, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	for _, entry := range plan.File {
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		assertSecretAbsent(t, data, secrets...)
	}
}

func TestWriteOnlySecretExports(t *testing.T) {
	for _, command := range []string{"generate", "genimport"} {
		t.Run(command, func(t *testing.T) {
			h := newHarness(t)
			const existing = "fixture-existing-secret"
			const replacement = "fixture-exported-secret"
			h.api.secretValues = map[string]any{"/v1/ai_secret": existing, "/v1/env_var": existing}
			config, stderr, code := h.run(exporterPath, command, "--resource-types", "braintrust_ai_secret,braintrust_env_var", "--org-id", org)
			if code != 0 {
				t.Fatalf("export failed:\n%s", stderr)
			}
			file, diags := hclsyntax.ParseConfig([]byte(config), "generated.tf", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatal(diags)
			}
			for _, block := range file.Body.(*hclsyntax.Body).Blocks {
				if block.Type == "variable" {
					h.env = append(h.env, "TF_VAR_"+block.Labels[0]+"="+replacement)
				}
			}
			h.writeFile("generated.tf", config)
			h.writeFile("provider.tf", providerConfig)
			stdout, stderr, code := h.run("terraform", "plan", "-input=false", "-no-color", "-out=export.tfplan")
			if code != 0 {
				t.Fatalf("export plan failed:\n%s%s", stdout, stderr)
			}
			assertSecretPlanAbsent(t, filepath.Join(h.workspace, "export.tfplan"), existing, replacement)
			stdout, stderr, code = h.run("terraform", "apply", "-input=false", "-no-color", "export.tfplan")
			if code != 0 {
				t.Fatalf("export apply failed:\n%s%s", stdout, stderr)
			}
			assertSecretAbsent(t, []byte(stdout+stderr), existing, replacement)
			assertSecretFileAbsent(t, filepath.Join(h.workspace, "terraform.tfstate"), existing, replacement)
			h.api.mu.Lock()
			writes, aiValue, envValue := h.api.secretWrites, h.api.secretValues["/v1/ai_secret"], h.api.secretValues["/v1/env_var"]
			h.api.mu.Unlock()
			wantWrites, wantValue := 2, replacement
			if command == "genimport" {
				wantWrites, wantValue = 0, existing
			}
			if writes != wantWrites || aiValue != wantValue || envValue != wantValue {
				t.Fatalf("%s changed secrets incorrectly: got %d writes, want %d", command, writes, wantWrites)
			}
			stdout, stderr, code = h.run("terraform", "plan", "-input=false", "-no-color", "-detailed-exitcode")
			if code != 0 {
				t.Fatalf("export was not stable after apply:\n%s%s", stdout, stderr)
			}
		})
	}
}
