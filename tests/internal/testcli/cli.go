// Package testcli runs provider integration commands in isolated workspaces.
package testcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Workspace configures both engines to use a local provider and fixture API.
// Development overrides intentionally bypass init and registry installation.
func Workspace(t *testing.T, binDir, apiURL string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	config := filepath.Join(root, "cli.tfrc")
	rc := fmt.Sprintf(`provider_installation {
  dev_overrides {
    "registry.terraform.io/braintrustdata/braintrust" = %q
    "registry.opentofu.org/braintrustdata/braintrust" = %q
  }
  direct {}
}
`, binDir, binDir)
	if err := os.WriteFile(config, []byte(rc), 0o600); err != nil {
		t.Fatal(err)
	}
	// Ambient CLI flags, credentials, reattach settings, and CLI configuration
	// must not change the scenario or direct requests outside the fixture API.
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "BRAINTRUST_") || strings.HasPrefix(name, "TF_") || strings.HasPrefix(name, "TOFU_") || name == "CHECKPOINT_DISABLE" {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"BRAINTRUST_API_KEY=offline-fixture-token",
		"BRAINTRUST_API_URL="+apiURL,
		"TF_IN_AUTOMATION=1",
		"CHECKPOINT_DISABLE=1",
		"TF_CLI_CONFIG_FILE="+config,
		"TOFU_CLI_CONFIG_FILE="+config,
	)
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	return workspace, env
}

// Run returns command output and its exit code. Launch failures and timeouts
// fail the test instead of being mistaken for expected application exits.
func Run(t *testing.T, dir string, env []string, name string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("run %s %v: %v\n%s%s", name, args, ctx.Err(), out.String(), errOut.String())
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return out.String(), errOut.String(), exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run %s %v: %v", name, args, err)
	}
	return out.String(), errOut.String(), 0
}
