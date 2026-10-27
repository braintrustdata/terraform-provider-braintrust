package offline

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExamples checks that each directory of .tf files under examples/ (the
// runnable examples/<name> configurations and the doc snippets) is formatted
// and valid against the current provider schema. It does not plan or apply them.
func TestExamples(t *testing.T) {
	root := filepath.Join("..", "..", "examples")
	dirs := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".tf" {
			return err
		}
		dir := filepath.Dir(path)
		dirs[dir] = append(dirs[dir], path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no examples found")
	}
	for dir, files := range dirs {
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			h := newHarness(t)
			declared := false
			for _, file := range files {
				src, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				h.writeFile(filepath.Base(file), string(src))
				declared = declared || strings.Contains(string(src), "required_providers")
			}
			// Doc snippets leave out the terraform block; without it the
			// provider would resolve to hashicorp/braintrust.
			if !declared {
				h.writeFile("versions_test.tf", "terraform {\n  required_providers {\n    braintrust = {\n      source = \"braintrustdata/braintrust\"\n    }\n  }\n}\n")
			}
			if out, errOut, code := h.run("terraform", "fmt", "-check", "-diff"); code != 0 {
				t.Fatalf("terraform fmt -check failed:\n%s%s", out, errOut)
			}
			if out, errOut, code := h.run("terraform", "validate", "-no-color"); code != 0 {
				t.Fatalf("terraform validate failed:\n%s%s", out, errOut)
			}
		})
	}
}
