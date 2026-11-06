package provider

import (
	"context"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = &apiKeyDataSource{}
	_ datasource.DataSourceWithConfigure = &apiKeyDataSource{}
)

// NewAPIKeyDataSource returns a metadata-only API key lookup.
func NewAPIKeyDataSource() datasource.DataSource { return &apiKeyDataSource{} }

type apiKeyDataSource struct{ client *client.Client }

type apiKeyDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	OrgID          types.String `tfsdk:"org_id"`
	Created        types.String `tfsdk:"created"`
	PreviewName    types.String `tfsdk:"preview_name"`
	UserID         types.String `tfsdk:"user_id"`
	UserEmail      types.String `tfsdk:"user_email"`
	UserGivenName  types.String `tfsdk:"user_given_name"`
	UserFamilyName types.String `tfsdk:"user_family_name"`
	ExpiresAt      types.String `tfsdk:"expires_at"`
}

func (d *apiKeyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (d *apiKeyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up Braintrust API key metadata by ID. The raw key is available only at creation and cannot be retrieved by this data source.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Required: true, Description: "Unique identifier for the API key (UUID)."},
			"name":             schema.StringAttribute{Computed: true, Description: "Name of the API key."},
			"org_id":           schema.StringAttribute{Computed: true, Description: "Organization ID of the API key."},
			"created":          schema.StringAttribute{Computed: true, Description: "Date of API key creation."},
			"preview_name":     schema.StringAttribute{Computed: true, Description: "Masked preview of the API key."},
			"user_id":          schema.StringAttribute{Computed: true, Description: "ID of the user who owns the API key."},
			"user_email":       schema.StringAttribute{Computed: true, Description: "Email of the user who owns the API key."},
			"user_given_name":  schema.StringAttribute{Computed: true, Description: "Given name of the user who owns the API key."},
			"user_family_name": schema.StringAttribute{Computed: true, Description: "Family name of the user who owns the API key."},
			"expires_at":       schema.StringAttribute{Computed: true, Description: "Actual expiration timestamp. Null when the key does not expire."},
		},
	}
}

func (d *apiKeyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *apiKeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config apiKeyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || !requireClient(d.client, &resp.Diagnostics) {
		return
	}
	id, err := uuid.Parse(config.ID.ValueString())
	if err != nil || id == uuid.Nil {
		resp.Diagnostics.AddAttributeError(path.Root("id"), "Invalid API Key ID", "id must be a known, nonzero API key UUID.")
		return
	}
	key, err := getAPIKey(ctx, d.client, id.String())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Braintrust API Key", apiKeyError(err))
		return
	}
	var metadata apiKeyModel
	mapAPIKeyToModel(key, &metadata)
	state := apiKeyDataSourceModel{
		ID: metadata.ID, Name: metadata.Name, OrgID: metadata.OrgID,
		Created: metadata.Created, PreviewName: metadata.PreviewName,
		UserID: metadata.UserID, UserEmail: metadata.UserEmail,
		UserGivenName: metadata.UserGivenName, UserFamilyName: metadata.UserFamilyName,
		ExpiresAt: metadata.ExpiresAt,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
