package cmd

import (
	"slices"
	"testing"
)

func TestACLsRequireExplicitSelection(t *testing.T) {
	defaults, err := selectResourceTypes([]string{"all"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(defaults, "braintrust_acl") {
		t.Fatal("all selected ACLs without opt-in")
	}
	if len(defaults) != len(AllResourceTypes)-1 {
		t.Fatalf("all selected %d types, want %d", len(defaults), len(AllResourceTypes)-1)
	}

	for _, test := range []struct {
		name        string
		types       []string
		includeACLs bool
	}{
		{"all with flag", []string{"all"}, true},
		{"explicit ACL", []string{"braintrust_project", "braintrust_acl"}, false},
		{"specific types with flag", []string{"braintrust_project"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			selected, err := selectResourceTypes(test.types, test.includeACLs)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(selected, "braintrust_acl") {
				t.Fatal("explicit ACL request was omitted")
			}
		})
	}

	selected, err := selectResourceTypes([]string{"braintrust_acl"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 {
		t.Fatalf("ACL selected %d times, want once", len(selected))
	}
}
