package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/hcl"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/schema"
	"github.com/spf13/cobra"
)

var genimportCmd = &cobra.Command{
	Use:   "genimport",
	Short: "Generate both Terraform resource configuration and import blocks",
	Long: `Fetches resources from the Braintrust API and generates both the Terraform
resource blocks (HCL) and the corresponding import blocks in a single output.
This is equivalent to running 'generate' followed by 'import'.`,
	RunE: runGenImport,
}

func init() {
	rootCmd.AddCommand(genimportCmd)
}

func runGenImport(cmd *cobra.Command, args []string) error {
	resourceTypes, err := resolveResourceTypes()
	if err != nil {
		return err
	}

	registry, err := schema.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load schema registry: %w", err)
	}

	ctx := context.Background()
	blocks, vars, dataSources, err := fetchAndBuildBlocks(ctx, resourceTypes)
	if err != nil {
		return err
	}
	blocks = importableBlocks(blocks)
	// The API never returns these values. Adoption must not plan a secret
	// rotation merely because recreation output contains placeholder variables.
	for i := range blocks {
		switch blocks[i].Type {
		case "braintrust_env_var":
			delete(blocks[i].LinkedFields, "value")
		case "braintrust_ai_secret":
			delete(blocks[i].LinkedFields, "secret")
		}
	}
	if len(vars) > 0 {
		fmt.Fprintln(os.Stderr, "Warning: omitted write-only values from adoption configuration; supply them explicitly when rotating or recreating secrets")
	}
	vars = nil

	writer := hcl.NewWriter(registry)
	output, shouldClose, err := getOutput()
	if err != nil {
		return err
	}
	if shouldClose {
		defer output.Close()
	}

	// Write data sources first so resources can reference them.
	if len(dataSources) > 0 {
		dataSourceHCL := writer.WriteDataSources(dataSources)
		if _, err := output.Write(dataSourceHCL); err != nil {
			return fmt.Errorf("failed to write data sources: %w", err)
		}
		if _, err := output.Write([]byte("\n")); err != nil {
			return fmt.Errorf("failed to write separator: %w", err)
		}
	}

	// Write resource blocks.
	data := writer.WriteResources(blocks)
	if _, err := output.Write(data); err != nil {
		return fmt.Errorf("failed to write resources: %w", err)
	}

	// Write variable blocks.
	if len(vars) > 0 {
		varData := writer.WriteVariables(vars)
		if _, err := output.Write(varData); err != nil {
			return fmt.Errorf("failed to write variables: %w", err)
		}
	}

	// Separator.
	if _, err := output.Write([]byte("\n# =============================================================================\n# Import blocks — run `terraform apply` to import existing resources into state.\n# =============================================================================\n\n")); err != nil {
		return fmt.Errorf("failed to write separator: %w", err)
	}

	// Write import blocks.
	importData := writer.WriteImports(blocks)
	if _, err := output.Write(importData); err != nil {
		return fmt.Errorf("failed to write imports: %w", err)
	}

	if shouldClose {
		fmt.Fprintf(os.Stderr, "Generated %d resource(s) + %d import block(s)\n", len(blocks), len(blocks))
	}

	return nil
}
