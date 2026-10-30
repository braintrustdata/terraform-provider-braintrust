package provider

import (
	"cmp"
	"context"
	"slices"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = &usersDataSource{}
	_ datasource.DataSourceWithConfigure = &usersDataSource{}
)

// usersFilterBatchSize caps how many emails or IDs go into one request so
// large filter sets don't exceed URL length limits.
const usersFilterBatchSize = 100

// NewUsersDataSource returns a new users data source.
func NewUsersDataSource() datasource.DataSource {
	return &usersDataSource{}
}

type usersDataSource struct {
	client *client.Client
}

// usersDataSourceModel is the Terraform data model for the users data source.
type usersDataSourceModel struct {
	Emails types.Set             `tfsdk:"emails"`
	IDs    types.Set             `tfsdk:"ids"`
	Users  []userDataSourceModel `tfsdk:"users"`
}

func (d *usersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

func (d *usersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to list Braintrust users, optionally filtered by email address or ID.",
		MarkdownDescription: "Use this data source to list Braintrust users, optionally filtered by email address or ID. " +
			"Unlike `braintrust_user`, emails or IDs that don't match a user are omitted from `users` rather than " +
			"failing, so it can be used to find which of a set of emails have Braintrust accounts.",
		Attributes: map[string]schema.Attribute{
			"emails": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Only return users with these email addresses. If unset, users are not filtered by email.",
			},
			"ids": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Only return users with these IDs (UUIDs). If unset, users are not filtered by ID.",
			},
			"users": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Matching users, sorted by email.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Unique identifier for the user (UUID).",
						},
						"email": schema.StringAttribute{
							Computed:    true,
							Description: "The user's email address.",
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
				},
			},
		},
	}
}

func (d *usersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *usersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config usersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(d.client, &resp.Diagnostics) {
		return
	}

	var emails, ids []string
	if !config.Emails.IsNull() {
		resp.Diagnostics.Append(config.Emails.ElementsAs(ctx, &emails, false)...)
	}
	if !config.IDs.IsNull() {
		resp.Diagnostics.Append(config.IDs.ElementsAs(ctx, &ids, false)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	userIDs, err := parseUUIDs(ids, "ids")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("ids"), "Invalid User ID", err.Error())
		return
	}

	users, err := d.listMatchingUsers(ctx, config, emails, userIDs)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Braintrust Users", "Could not list users: "+err.Error())
		return
	}

	// Filter client-side too, so results only ever contain requested users.
	emailSet := make(map[string]bool, len(emails))
	for _, email := range emails {
		emailSet[email] = true
	}
	idSet := make(map[string]bool, len(ids))
	for _, id := range userIDs {
		idSet[id.String()] = true
	}
	state := usersDataSourceModel{Emails: config.Emails, IDs: config.IDs, Users: []userDataSourceModel{}}
	seen := map[string]bool{}
	for i := range users {
		user := &users[i]
		if !config.Emails.IsNull() && (!nullableIsSet(user.Email) || !emailSet[user.Email.MustGet()]) {
			continue
		}
		if !config.IDs.IsNull() && !idSet[user.Id.String()] {
			continue
		}
		if seen[user.Id.String()] {
			continue
		}
		seen[user.Id.String()] = true
		state.Users = append(state.Users, userToModel(user))
	}
	slices.SortFunc(state.Users, func(a, b userDataSourceModel) int {
		return cmp.Or(cmp.Compare(a.Email.ValueString(), b.Email.ValueString()), cmp.Compare(a.ID.ValueString(), b.ID.ValueString()))
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// listMatchingUsers lists users matching the configured filters. Large
// filter sets are split across requests to stay under URL length limits: the
// email filter is batched when set, otherwise the ID filter is. An explicitly
// empty filter set matches no users.
func (d *usersDataSource) listMatchingUsers(ctx context.Context, config usersDataSourceModel, emails []string, ids []uuid.UUID) ([]openapi.User, error) {
	if (!config.Emails.IsNull() && len(emails) == 0) || (!config.IDs.IsNull() && len(ids) == 0) {
		return nil, nil
	}
	var requests []openapi.GetUserParams
	switch {
	case len(emails) > 0:
		for batch := range slices.Chunk(emails, usersFilterBatchSize) {
			params := openapi.GetUserParams{Email: &openapi.UserEmail{}}
			_ = params.Email.FromUserEmail1(batch) // marshaling a []string can't fail
			if len(ids) > 0 {
				params.Ids = idsFilter(ids)
			}
			requests = append(requests, params)
		}
	case len(ids) > 0:
		for batch := range slices.Chunk(ids, usersFilterBatchSize) {
			requests = append(requests, openapi.GetUserParams{Ids: idsFilter(batch)})
		}
	default:
		requests = []openapi.GetUserParams{{}}
	}

	var users []openapi.User
	for _, params := range requests {
		page, err := listUsers(ctx, d.client, params)
		if err != nil {
			return nil, err
		}
		users = append(users, page...)
	}
	return users, nil
}

func idsFilter(ids []uuid.UUID) *openapi.Ids {
	var filter openapi.Ids
	_ = filter.FromIds1(ids) // marshaling a []uuid.UUID can't fail
	return &filter
}
