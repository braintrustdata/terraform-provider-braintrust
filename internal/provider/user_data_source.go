package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ datasource.DataSource              = &userDataSource{}
	_ datasource.DataSourceWithConfigure = &userDataSource{}
)

// NewUserDataSource returns a new user data source.
func NewUserDataSource() datasource.DataSource {
	return &userDataSource{}
}

type userDataSource struct {
	client *client.Client
}

// userDataSourceModel is the Terraform data model for the user data source.
type userDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Email      types.String `tfsdk:"email"`
	GivenName  types.String `tfsdk:"given_name"`
	FamilyName types.String `tfsdk:"family_name"`
	AvatarURL  types.String `tfsdk:"avatar_url"`
	Created    types.String `tfsdk:"created"`
}

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Braintrust user by email address.",
		MarkdownDescription: "Use this data source to look up a Braintrust user by email address. " +
			"This is useful for referencing users when managing group membership or ACLs.\n\n" +
			"```hcl\ndata \"braintrust_user\" \"alice\" {\n  email = \"alice@company.com\"\n}\n\n" +
			"resource \"braintrust_group\" \"team\" {\n  name         = \"My Team\"\n  member_users = [data.braintrust_user.alice.id]\n}\n```",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Unique identifier for the user (UUID). Provide either `id` or `email`.",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("email")),
				},
			},
			"email": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The user's email address. Provide either `email` or `id`.",
			},
			"given_name": schema.StringAttribute{
				Computed:    true,
				Description: "Given (first) name of the user.",
			},
			"family_name": schema.StringAttribute{
				Computed:    true,
				Description: "Family (last) name of the user.",
			},
			"avatar_url": schema.StringAttribute{
				Computed:    true,
				Description: "URL of the user's avatar image.",
			},
			"created": schema.StringAttribute{
				Computed:    true,
				Description: "Date of user creation.",
			},
		},
	}
}

func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config userDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(d.client, &resp.Diagnostics) {
		return
	}

	var users []openapi.User
	var err error
	var lookupKey string

	if !config.Email.IsNull() && !config.Email.IsUnknown() {
		email := config.Email.ValueString()
		lookupKey = email
		tflog.Debug(ctx, "Looking up user by email", map[string]any{"email": email})
		var filter openapi.UserEmail
		if err = filter.FromUserEmail0(email); err == nil {
			users, err = listUsers(ctx, d.client, openapi.GetUserParams{Email: &filter})
		}
	} else if !config.ID.IsNull() && !config.ID.IsUnknown() {
		id := config.ID.ValueString()
		lookupKey = id
		tflog.Debug(ctx, "Looking up user by ID", map[string]any{"id": id})
		var userID uuid.UUID
		if userID, err = parseUUID(config.ID, "id"); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("id"), "Invalid User ID", err.Error())
			return
		}
		var filter openapi.Ids
		if err = filter.FromIds0(userID); err == nil {
			users, err = listUsers(ctx, d.client, openapi.GetUserParams{Ids: &filter})
		}
	} else {
		resp.Diagnostics.AddError(
			"Missing Lookup Key",
			"Either 'email' or 'id' must be specified.",
		)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust User",
			"Could not list users: "+err.Error(),
		)
		return
	}

	var matches []openapi.User
	for _, candidate := range users {
		if !config.Email.IsNull() && !config.Email.IsUnknown() && (!nullableIsSet(candidate.Email) || candidate.Email.MustGet() != config.Email.ValueString()) {
			continue
		}
		if !config.ID.IsNull() && !config.ID.IsUnknown() && candidate.Id.String() != config.ID.ValueString() {
			continue
		}
		matches = append(matches, candidate)
	}
	if len(matches) == 0 {
		resp.Diagnostics.AddError(
			"User Not Found",
			"No user found matching: "+lookupKey,
		)
		return
	}
	if len(matches) > 1 {
		resp.Diagnostics.AddError("Ambiguous User", "Multiple users matched "+lookupKey+". Look up a user by ID.")
		return
	}

	user := &matches[0]
	var state userDataSourceModel
	state.ID = types.StringValue(user.Id.String())
	state.Email = nullableStringToTF(user.Email)
	state.GivenName = nullableStringToTF(user.GivenName)
	state.FamilyName = nullableStringToTF(user.FamilyName)
	state.AvatarURL = nullableStringToTF(user.AvatarUrl)
	state.Created = nullableTimeToTF(user.Created, types.StringNull())

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// listUsers lists users matching the filters in params, across all pages.
func listUsers(ctx context.Context, c *client.Client, params openapi.GetUserParams) ([]openapi.User, error) {
	return client.ListObjects("/v1/user", func(limit int, startingAfter *string) (*client.Page[openapi.User], error) {
		params.Limit, params.StartingAfter = &limit, startingAfter
		page, err := c.API.GetUserWithResponse(ctx, &params)
		if err != nil {
			return nil, err
		}
		return page.JSON200, nil
	}, func(user openapi.User) string { return user.Id.String() })
}
