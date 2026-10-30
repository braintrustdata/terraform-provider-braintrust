package provider

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource                     = &usersDataSource{}
	_ datasource.DataSourceWithConfigure        = &usersDataSource{}
	_ datasource.DataSourceWithConfigValidators = &usersDataSource{}
	_ datasource.DataSourceWithValidateConfig   = &usersDataSource{}
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
	Emails        types.Set             `tfsdk:"emails"`
	IDs           types.Set             `tfsdk:"ids"`
	ReturnAll     types.Bool            `tfsdk:"return_all"`
	IgnoreMissing types.Bool            `tfsdk:"ignore_missing"`
	Users         []userDataSourceModel `tfsdk:"users"`
	MissingEmails []string              `tfsdk:"missing_emails"`
	MissingIDs    []string              `tfsdk:"missing_ids"`
}

func (d *usersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

func (d *usersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up many Braintrust users at once by email address or ID, or to list every user.",
		MarkdownDescription: "Use this data source to look up many Braintrust users at once by email address or ID, or to list every user. " +
			"Set exactly one of `emails`, `ids`, or `return_all`. Like `braintrust_user`, it fails if a requested " +
			"email or ID doesn't match a user, unless `ignore_missing` is set.",
		Attributes: map[string]schema.Attribute{
			"emails": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Look up users with these email addresses.",
			},
			"ids": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Look up users with these IDs (UUIDs).",
			},
			"return_all": schema.BoolAttribute{
				Optional:    true,
				Description: "Set to `true` to return every user in the organization.",
			},
			"ignore_missing": schema.BoolAttribute{
				Optional: true,
				Description: "If `true`, emails or IDs that don't match a user are listed in `missing_emails` or " +
					"`missing_ids` instead of failing. Defaults to `false`.",
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
			"missing_emails": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Requested emails that don't match a user. Only non-empty when `ignore_missing` is `true`.",
			},
			"missing_ids": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Requested IDs that don't match a user. Only non-empty when `ignore_missing` is `true`.",
			},
		},
	}
}

func (d *usersDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("emails"), path.MatchRoot("ids"), path.MatchRoot("return_all")),
	}
}

func (d *usersDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var returnAll types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("return_all"), &returnAll)...)
	if !returnAll.IsNull() && !returnAll.IsUnknown() && !returnAll.ValueBool() {
		resp.Diagnostics.AddAttributeError(path.Root("return_all"), "Invalid return_all",
			"return_all must be true when set. To look up specific users, set emails or ids instead.")
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

	users, err := d.listMatchingUsers(ctx, emails, userIDs, config.ReturnAll.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Braintrust Users", "Could not list users: "+err.Error())
		return
	}

	// Filter client-side too, so results only ever contain requested users,
	// and track which requested emails and IDs were found.
	foundEmails := make(map[string]bool, len(emails))
	for _, email := range emails {
		foundEmails[email] = false
	}
	foundIDs := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		foundIDs[id.String()] = false
	}
	state := config
	state.Users = []userDataSourceModel{}
	seen := map[string]bool{}
	for i := range users {
		user := &users[i]
		id := user.Id.String()
		if !config.Emails.IsNull() {
			if !nullableIsSet(user.Email) {
				continue
			}
			if _, ok := foundEmails[user.Email.MustGet()]; !ok {
				continue
			}
			foundEmails[user.Email.MustGet()] = true
		}
		if !config.IDs.IsNull() {
			if _, ok := foundIDs[id]; !ok {
				continue
			}
			foundIDs[id] = true
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		state.Users = append(state.Users, userToModel(user))
	}
	slices.SortFunc(state.Users, func(a, b userDataSourceModel) int {
		return cmp.Or(cmp.Compare(a.Email.ValueString(), b.Email.ValueString()), cmp.Compare(a.ID.ValueString(), b.ID.ValueString()))
	})

	state.MissingEmails = missingKeys(foundEmails)
	state.MissingIDs = missingKeys(foundIDs)
	if !config.IgnoreMissing.ValueBool() {
		if len(state.MissingEmails) > 0 {
			resp.Diagnostics.AddAttributeError(path.Root("emails"), "Users Not Found",
				"No user found for: "+strings.Join(state.MissingEmails, ", ")+". Set ignore_missing = true to skip them.")
		}
		if len(state.MissingIDs) > 0 {
			resp.Diagnostics.AddAttributeError(path.Root("ids"), "Users Not Found",
				"No user found for: "+strings.Join(state.MissingIDs, ", ")+". Set ignore_missing = true to skip them.")
		}
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// listMatchingUsers lists users with the given emails or IDs, or every user
// if returnAll is set. Large filter sets are split across requests to stay
// under URL length limits. An empty filter set matches no users.
func (d *usersDataSource) listMatchingUsers(ctx context.Context, emails []string, ids []uuid.UUID, returnAll bool) ([]openapi.User, error) {
	var requests []openapi.GetUserParams
	switch {
	case returnAll:
		requests = []openapi.GetUserParams{{}}
	case len(emails) > 0:
		for batch := range slices.Chunk(emails, usersFilterBatchSize) {
			params := openapi.GetUserParams{Email: &openapi.UserEmail{}}
			_ = params.Email.FromUserEmail1(batch) // marshaling a []string can't fail
			requests = append(requests, params)
		}
	case len(ids) > 0:
		for batch := range slices.Chunk(ids, usersFilterBatchSize) {
			var filter openapi.Ids
			_ = filter.FromIds1(batch) // marshaling a []uuid.UUID can't fail
			requests = append(requests, openapi.GetUserParams{Ids: &filter})
		}
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

// missingKeys returns the sorted keys of found whose value is false.
func missingKeys(found map[string]bool) []string {
	missing := []string{}
	for key, ok := range found {
		if !ok {
			missing = append(missing, key)
		}
	}
	slices.Sort(missing)
	return missing
}
