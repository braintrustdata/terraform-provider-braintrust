package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ provider.Provider                       = &braintrustProvider{}
	_ provider.ProviderWithEphemeralResources = &braintrustProvider{}
)

// Terraform 1.11.1 fixes serialization of values marked sensitive and ephemeral.
var minimumTerraformVersion = version.Must(version.NewSemver("1.11.1"))

// braintrustProvider is the provider implementation.
type braintrustProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// braintrustProviderModel describes the provider data model.
type braintrustProviderModel struct {
	APIKey types.String `tfsdk:"api_key"`
	APIURL types.String `tfsdk:"api_url"`
}

// New returns a new provider factory function.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &braintrustProvider{
			version: version,
		}
	}
}

// Metadata returns the provider type name.
func (p *braintrustProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "braintrust"
	resp.Version = p.version
}

// Schema defines the provider-level schema.
func (p *braintrustProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "The Braintrust provider allows you to manage Braintrust resources. Requires Terraform 1.11.1 or later.",
		MarkdownDescription: "The Braintrust provider allows you to manage [Braintrust](https://www.braintrust.dev/) resources such as projects, datasets, functions, access control, and automations. Requires Terraform 1.11.1 or later.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				Description:         "The API key for authenticating with the Braintrust API. Can also be set via the BRAINTRUST_API_KEY environment variable.",
				MarkdownDescription: "The API key for authenticating with the Braintrust API. Can also be set via the `BRAINTRUST_API_KEY` environment variable.",
			},
			"api_url": schema.StringAttribute{
				Optional:            true,
				Description:         "The base URL of the Braintrust API. Defaults to https://api.braintrust.dev. Can also be set via the BRAINTRUST_API_URL environment variable.",
				MarkdownDescription: "The base URL of the Braintrust API. Defaults to `https://api.braintrust.dev`. Can also be set via the `BRAINTRUST_API_URL` environment variable.",
			},
		},
	}
}

// Configure prepares a Braintrust API client for data sources and resources.
func (p *braintrustProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	terraformVersion, err := version.NewSemver(req.TerraformVersion)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Terraform Version",
			fmt.Sprintf("The Braintrust provider requires Terraform %s or later, but received an invalid or unknown Terraform version %q. Run this provider with Terraform %s or later.", minimumTerraformVersion, req.TerraformVersion, minimumTerraformVersion),
		)
		return
	}
	if terraformVersion.LessThan(minimumTerraformVersion) {
		resp.Diagnostics.AddError(
			"Unsupported Terraform Version",
			fmt.Sprintf("The Braintrust provider requires Terraform %s or later, but Terraform %q is running. Upgrade Terraform to %s or later.", minimumTerraformVersion, req.TerraformVersion, minimumTerraformVersion),
		)
		return
	}

	var config braintrustProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Determine API key: config takes precedence over environment variable.
	apiKey := os.Getenv(client.EnvAPIKey)
	if !config.APIKey.IsNull() && !config.APIKey.IsUnknown() {
		apiKey = config.APIKey.ValueString()
	}

	if apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing API Key",
			"The provider requires a Braintrust API key. Set it in the provider configuration block or via the "+client.EnvAPIKey+" environment variable.",
		)
		return
	}

	// Determine API URL: config takes precedence over environment variable.
	apiURL := os.Getenv(client.EnvAPIURL)
	if !config.APIURL.IsNull() && !config.APIURL.IsUnknown() {
		apiURL = config.APIURL.ValueString()
	}

	// Create the API client.
	userAgent := "terraform-provider-braintrust/" + p.version
	c, err := client.NewClient(apiURL, apiKey, userAgent)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Braintrust API Client",
			"An unexpected error occurred when creating the Braintrust API client: "+err.Error(),
		)
		return
	}

	// Fail during plan, before any write, if the org lives on another data plane.
	if err := c.CheckDataPlane(ctx); err != nil {
		resp.Diagnostics.AddError("Wrong Braintrust Data Plane", err.Error())
		return
	}

	// Make the client available to data sources and resources.
	resp.DataSourceData = c
	resp.ResourceData = c
	resp.EphemeralResourceData = c
}

// Resources defines the resources implemented in the provider.
func (p *braintrustProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProjectResource,
		NewDatasetResource,
		NewFunctionResource,
		NewGroupResource,
		NewGroupMembershipResource,
		NewProjectGroupResource,
		NewProjectGroupMembershipResource,
		NewRoleResource,
		NewACLResource,
		NewEnvVarResource,
		NewProjectTagResource,
		NewProjectScoreResource,
		NewServiceAccountResource,
		NewServiceTokenResource,
		NewAISecretResource,
		NewProjectAutomationResource,
		NewMCPServerResource,
		NewViewResource,
	}
}

// EphemeralResources defines state-free resources implemented in the provider.
func (p *braintrustProvider) EphemeralResources(_ context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{
		NewServiceTokenEphemeralResource,
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *braintrustProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProjectDataSource,
		NewDatasetDataSource,
		NewFunctionDataSource,
		NewGroupDataSource,
		NewProjectGroupDataSource,
		NewRoleDataSource,
		NewUserDataSource,
		NewUsersDataSource,
	}
}
