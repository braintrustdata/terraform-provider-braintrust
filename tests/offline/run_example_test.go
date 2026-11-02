package offline

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEphemeralServiceTokenExample(t *testing.T) {
	h := newHarness(t)
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// Keep the real make target, runner, build, provider and Terraform in the
	// execution path. The fixture server's unique port isolates its state.
	var apiURL string
	env := h.env[:0]
	for _, kv := range h.env {
		name, value, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "TF_") || name == "MAKEFLAGS" || name == "MFLAGS" {
			continue
		}
		if name == "BRAINTRUST_API_URL" {
			apiURL = value
		}
		env = append(env, kv)
	}
	h.env = env
	envDir := filepath.Join(repo, ".example-state", strings.ReplaceAll(strings.TrimPrefix(apiURL, "http://"), ":", "_"))
	h.workspace = filepath.Join(envDir, "ephemeral-service-token")
	t.Cleanup(func() { _ = os.RemoveAll(envDir) })
	var outputs []string
	run := func(mode string) {
		t.Helper()
		command := *h
		command.workspace = repo
		stdout, stderr, code := command.run("make", "run-example", "MODE="+mode, "examples/ephemeral-service-token")
		outputs = append(outputs, stdout, stderr)
		h.assertTokenArtifactsAbsent(outputs)
		if code != 0 {
			t.Fatalf("runner %s failed:\n%s%s", mode, stdout, stderr)
		}
		t.Logf("make run-example MODE=%s examples/ephemeral-service-token succeeded", mode)
	}

	run("plan")
	h.assertTokenCounts(0, 0)
	// Even a plan with no state must preserve interrupted delivery files.
	recovery := filepath.Join(h.workspace, ".secrets", ".service-token-recovery")
	if err := os.MkdirAll(filepath.Dir(recovery), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recovery, []byte("private-recovery-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("plan")
	data, err := os.ReadFile(recovery)
	if err != nil || string(data) != "private-recovery-data" {
		t.Fatal("plan discarded interrupted delivery files")
	}
	if err := os.Remove(recovery); err != nil {
		t.Fatal(err)
	}
	run("apply")
	h.assertTokenCounts(1, 0)
	original := h.assertStoredToken(0)
	for _, mode := range []string{"plan", "apply"} {
		run(mode)
		h.assertTokenCounts(1, 0)
		if !bytes.Equal(original, h.assertStoredToken(0)) {
			t.Fatalf("%s changed the stored credential", mode)
		}
	}

	run("destroy")
	h.assertTokenCounts(1, 1)
	if _, err := os.Stat(h.workspace); !os.IsNotExist(err) {
		t.Fatal("successful destroy left the demo workspace or secret store behind")
	}
	run("plan")
	h.assertTokenCounts(1, 1)
	run("apply-then-destroy")
	h.assertTokenCounts(2, 2)
	if _, err := os.Stat(h.workspace); !os.IsNotExist(err) {
		t.Fatal("apply-then-destroy left the demo workspace or secret store behind")
	}
	t.Log("plans minted no tokens; repeat runs reused the store; destroy revoked tokens and deleted keys; state and output contained no token keys")
}
