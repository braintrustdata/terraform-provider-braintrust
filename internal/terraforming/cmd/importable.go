package cmd

import (
	"fmt"
	"os"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/hcl"
)

// The provider has no ImportState implementation for these resources. Emitting
// import blocks for them makes Terraform fail before it can import anything.
func importableBlocks(blocks []hcl.ResourceBlock) []hcl.ResourceBlock {
	var result []hcl.ResourceBlock
	skipped := map[string]int{}
	for _, block := range blocks {
		if block.Type == "braintrust_service_token" {
			skipped[block.Type]++
			continue
		}
		result = append(result, block)
	}
	for _, resourceType := range []string{"braintrust_service_token"} {
		if skipped[resourceType] > 0 {
			fmt.Fprintf(os.Stderr, "Warning: omitted %d %s resource(s): provider does not support import for this type\n", skipped[resourceType], resourceType)
		}
	}
	return result
}
