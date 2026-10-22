package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/hcl"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/schema"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Generate Terraform import blocks for existing Braintrust resources",
	Long: `Fetches resources from the Braintrust API and generates Terraform 1.5+
import blocks. These can be placed in a .tf file alongside the generated
resource configuration to import existing state.`,
	RunE: runImport,
}

func init() {
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	resourceTypes, err := resolveResourceTypes()
	if err != nil {
		return err
	}

	registry, err := schema.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load schema registry: %w", err)
	}

	ctx := context.Background()
	blocks, _, _, err := fetchAndBuildBlocks(ctx, resourceTypes)
	if err != nil {
		return err
	}
	blocks = importableBlocks(blocks)

	writer := hcl.NewWriter(registry)
	output, shouldClose, err := getOutput()
	if err != nil {
		return err
	}
	if shouldClose {
		defer output.Close()
	}

	data := writer.WriteImports(blocks)
	if _, err := output.Write(data); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	if shouldClose {
		fmt.Fprintf(os.Stderr, "Generated %d import block(s)\n", len(blocks))
	}

	return nil
}
