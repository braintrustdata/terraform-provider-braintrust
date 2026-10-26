package cmd

import (
	"fmt"
	"os"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version   = "dev"
	apiClient *api.Client
)

// SetVersion sets the version string (called from main).
func SetVersion(v string) {
	version = v
}

var rootCmd = &cobra.Command{
	Use:   "braintrust-terraforming",
	Short: "Generate Terraform configuration from an existing Braintrust org",
	Long: `braintrust-terraforming fetches resources from the Braintrust API and
generates the corresponding Terraform configuration files and import blocks.

Use it to bootstrap Terraform management of an existing Braintrust deployment.`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip client init for version command.
		if cmd.Name() == "version" {
			return nil
		}

		token := viper.GetString("token")
		if token == "" {
			return fmt.Errorf("API token is required. Set --token or BRAINTRUST_API_KEY environment variable")
		}

		apiURL := viper.GetString("api-url")
		userAgent := fmt.Sprintf("braintrust-terraforming/%s", version)

		var err error
		apiClient, err = api.NewClient(apiURL, token, userAgent)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		return nil
	},
}

func init() {
	// Persistent flags available to all subcommands.
	rootCmd.PersistentFlags().StringP("token", "t", "", "Braintrust API key [env: BRAINTRUST_API_KEY]")
	rootCmd.PersistentFlags().String("api-url", api.DefaultBaseURL, "Braintrust API base URL [env: BRAINTRUST_API_URL]")
	rootCmd.PersistentFlags().String("org-id", "", "Organization ID to export [env: BRAINTRUST_ORG_ID]; required if the credential can see multiple or no projects")
	rootCmd.PersistentFlags().StringSlice("resource-types", nil, "Resource types to generate (comma-separated, or 'all'; ACLs excluded from 'all')")
	rootCmd.PersistentFlags().Bool("include-acls", false, "Include braintrust_acl in the selected resource types (ACLs are excluded from 'all' by default)")
	rootCmd.PersistentFlags().StringSlice("projects", nil, "Project IDs or names to scope to")
	rootCmd.PersistentFlags().StringSlice("linked-resource-types", nil, "Resource types to emit as Terraform references instead of raw IDs")
	rootCmd.PersistentFlags().Bool("no-linked-resources", false, "Disable linked resource references; emit raw IDs for all relationships")
	rootCmd.PersistentFlags().Bool("strict-user-linking", false, "Fail generation if any group member user ID cannot be resolved to an email data source")
	rootCmd.PersistentFlags().Bool("verify-acl-targets", true, "Read each ACL individually to omit grants with missing or inaccessible targets (use =false for list-only export)")
	rootCmd.PersistentFlags().StringP("output", "o", "", "Output file path (default: stdout)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")

	// Bind flags to viper for env var support.
	_ = viper.BindPFlag("token", rootCmd.PersistentFlags().Lookup("token"))
	_ = viper.BindPFlag("api-url", rootCmd.PersistentFlags().Lookup("api-url"))
	_ = viper.BindPFlag("org-id", rootCmd.PersistentFlags().Lookup("org-id"))
	_ = viper.BindPFlag("resource-types", rootCmd.PersistentFlags().Lookup("resource-types"))
	_ = viper.BindPFlag("include-acls", rootCmd.PersistentFlags().Lookup("include-acls"))
	_ = viper.BindPFlag("projects", rootCmd.PersistentFlags().Lookup("projects"))
	_ = viper.BindPFlag("linked-resource-types", rootCmd.PersistentFlags().Lookup("linked-resource-types"))
	_ = viper.BindPFlag("no-linked-resources", rootCmd.PersistentFlags().Lookup("no-linked-resources"))
	_ = viper.BindPFlag("strict-user-linking", rootCmd.PersistentFlags().Lookup("strict-user-linking"))
	_ = viper.BindPFlag("verify-acl-targets", rootCmd.PersistentFlags().Lookup("verify-acl-targets"))
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	// Environment variable bindings.
	_ = viper.BindEnv("token", "BRAINTRUST_API_KEY")
	_ = viper.BindEnv("api-url", "BRAINTRUST_API_URL")
	_ = viper.BindEnv("org-id", "BRAINTRUST_ORG_ID")
}

// DefaultLinkedResourceTypes are emitted as Terraform references when
// --linked-resource-types is not provided.
var DefaultLinkedResourceTypes = []string{
	"braintrust_project",
	"braintrust_role",
	"braintrust_group",
	"braintrust_project_group",
	"braintrust_project_group_membership",
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// getOutput returns a writer for the output (file or stdout).
func getOutput() (*os.File, bool, error) {
	outputPath := viper.GetString("output")
	if outputPath == "" {
		return os.Stdout, false, nil
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create output file %s: %w", outputPath, err)
	}
	return f, true, nil
}

// AllResourceTypes is the catalog of supported resource types, including ACLs.
var AllResourceTypes = []string{
	"braintrust_project",
	"braintrust_role",
	"braintrust_group",
	"braintrust_group_membership",
	"braintrust_project_group",
	"braintrust_project_group_membership",
	"braintrust_acl",
	"braintrust_dataset",
	"braintrust_function",
	"braintrust_project_score",
	"braintrust_project_tag",
	"braintrust_project_automation",
	"braintrust_org_automation",
	"braintrust_mcp_server",
	"braintrust_env_var",
	"braintrust_service_token",
	"braintrust_ai_secret",
	"braintrust_view",
}

// resolveResourceTypes expands "all" to the default set and validates explicit
// selections. ACLs require an explicit resource type or --include-acls.
func resolveResourceTypes() ([]string, error) {
	types := viper.GetStringSlice("resource-types")
	includeACLs := viper.GetBool("include-acls")
	selected, err := selectResourceTypes(types, includeACLs)
	if err != nil {
		return nil, err
	}
	if len(types) == 1 && types[0] == "all" && !includeACLs {
		fmt.Fprintln(os.Stderr, "Note: --resource-types all excludes braintrust_acl; use --include-acls to export direct ACLs")
	}
	return selected, nil
}

func selectResourceTypes(types []string, includeACLs bool) ([]string, error) {
	if len(types) == 0 {
		return nil, fmt.Errorf("--resource-types is required (use 'all' for the default resource set)")
	}
	if len(types) == 1 && types[0] == "all" {
		selected := make([]string, 0, len(AllResourceTypes))
		for _, t := range AllResourceTypes {
			if t != "braintrust_acl" || includeACLs {
				selected = append(selected, t)
			}
		}
		return selected, nil
	}
	valid := make(map[string]bool)
	for _, t := range AllResourceTypes {
		valid[t] = true
	}
	for _, t := range types {
		if !valid[t] {
			return nil, fmt.Errorf("unknown resource type: %q (valid types: %v)", t, AllResourceTypes)
		}
	}
	if includeACLs {
		for _, t := range types {
			if t == "braintrust_acl" {
				return types, nil
			}
		}
		return append(types, "braintrust_acl"), nil
	}
	return types, nil
}
