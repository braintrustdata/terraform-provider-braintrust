package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestEnvMetadataLifecycle(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000001"
	const objectID = "00000000-0000-0000-0000-000000000002"
	const initialMetadata = `{ "future": {"integer":9007199254740993,"decimal":0.1234567890123456789}, "removed":true }`
	const replacementMetadata = `{"future":{"integer":9007199254740995},"replacement":true}`
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprintf("configured_%t", configured), func(t *testing.T) {
			ctx := context.Background()
			var remoteMetadata json.RawMessage
			var requests []map[string]json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.Method == http.MethodPut && req.URL.Path == "/v1/env_var",
					req.Method == http.MethodPatch && req.URL.Path == "/v1/env_var/"+id:
					var fields map[string]json.RawMessage
					if err := json.NewDecoder(req.Body).Decode(&fields); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					requests = append(requests, fields)
					if metadata, ok := fields["metadata"]; ok {
						remoteMetadata = metadata
					}
				case req.Method == http.MethodGet && req.URL.Path == "/v1/env_var/"+id:
				default:
					t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				metadata := remoteMetadata
				if metadata == nil {
					metadata = json.RawMessage("null")
				}
				_, _ = fmt.Fprintf(w, `{"id":%q,"object_id":%q,"object_type":"project","name":"key","metadata":%s,"value":"must-not-persist"}`, id, objectID, metadata)
			}))
			defer server.Close()
			c, err := client.NewClient(server.URL, "fixture", "")
			if err != nil {
				t.Fatal(err)
			}
			r := &envVarResource{client: c}
			var schemaResp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			attribute := schemaResp.Schema.Attributes["metadata"].(schema.StringAttribute)
			if !attribute.Optional || !attribute.Computed || !attribute.Sensitive || attribute.WriteOnly {
				t.Fatal("metadata must be optional, computed, sensitive, and persisted")
			}
			stateFor := func(model envVarModel) tfsdk.State {
				state := tfsdk.State{Schema: schemaResp.Schema}
				if diags := state.Set(ctx, model); diags.HasError() {
					t.Fatal(diags)
				}
				return state
			}
			modelFrom := func(state tfsdk.State) envVarModel {
				var model envVarModel
				if diags := state.Get(ctx, &model); diags.HasError() {
					t.Fatal(diags)
				}
				if !model.Value.IsNull() {
					t.Fatal("write-only value persisted in state")
				}
				return model
			}
			plan := envVarModel{
				ObjectType: types.StringValue("project"), ObjectID: types.StringValue(objectID),
				Name: types.StringValue("key"), ValueVersion: types.Int64Value(1),
			}
			// Terraform plans an omitted Optional+Computed attribute as unknown.
			plan.Metadata = types.StringUnknown()
			wantMetadata := types.StringNull()
			if configured {
				plan.Metadata = types.StringValue(initialMetadata)
				wantMetadata = plan.Metadata
			}
			config := plan
			config.Value = types.StringValue("initial-secret")
			createResp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			r.Create(ctx, resource.CreateRequest{
				Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: stateFor(plan).Raw},
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: stateFor(config).Raw},
			}, &createResp)
			if createResp.Diagnostics.HasError() {
				t.Fatal(createResp.Diagnostics)
			}
			if len(requests) != 1 || string(requests[0]["value"]) != `"initial-secret"` {
				t.Fatalf("unexpected create body: %v", requests)
			}
			if _, present := requests[0]["metadata"]; present != configured {
				t.Fatalf("metadata presence = %t, want %t", present, configured)
			}
			state := createResp.State
			model := modelFrom(state)
			if !model.Metadata.Equal(wantMetadata) || !model.ValueVersion.Equal(plan.ValueVersion) {
				t.Fatal("create changed metadata spelling/precision or value version")
			}
			updates := []struct {
				name     string
				metadata types.String
				present  bool
				body     string
				want     types.String
			}{
				{"replace", types.StringValue(replacementMetadata), true, replacementMetadata, types.StringValue(replacementMetadata)},
				{"unchanged", types.StringValue(`{ "replacement": true, "future": {"integer": 9007199254740995} }`), false, "", types.StringValue(`{ "replacement": true, "future": {"integer": 9007199254740995} }`)},
				// 9007199254740995 and ...996 are the same float64, which is all the API keeps.
				{"float64_rounding", types.StringValue(`{"future":{"integer":9007199254740996},"replacement":true}`), false, "", types.StringValue(`{"future":{"integer":9007199254740996},"replacement":true}`)},
				{"drift", types.StringValue(`{"future":{"integer":7},"replacement":true}`), true, `{"future":{"integer":7},"replacement":true}`, types.StringValue(`{"future":{"integer":7},"replacement":true}`)},
				{"clear", types.StringValue(`{}`), true, "{}", types.StringValue(`{}`)},
				{"omitted", types.StringUnknown(), false, "", types.StringNull()},
			}
			for _, update := range updates {
				plan = modelFrom(state)
				plan.Metadata = update.metadata
				config = plan
				config.Value = types.StringValue("do-not-resend")
				updateResp := resource.UpdateResponse{State: state}
				r.Update(ctx, resource.UpdateRequest{
					State:  state,
					Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: stateFor(plan).Raw},
					Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: stateFor(config).Raw},
				}, &updateResp)
				if updateResp.Diagnostics.HasError() {
					t.Fatalf("%s: %v", update.name, updateResp.Diagnostics)
				}
				body := requests[len(requests)-1]
				metadata, present := body["metadata"]
				if present != update.present || string(metadata) != update.body {
					t.Fatalf("%s: metadata body = %s, present = %t", update.name, metadata, present)
				}
				if _, present := body["value"]; present {
					t.Fatalf("%s: metadata update resent the secret", update.name)
				}
				state = updateResp.State
				model = modelFrom(state)
				if !model.Metadata.Equal(update.want) || !model.ValueVersion.Equal(types.Int64Value(1)) {
					t.Fatalf("%s: metadata or version not preserved", update.name)
				}
			}

			// Import begins with only an ID, then refresh discovers metadata without
			// narrowing unknown fields or rounding large numbers through SDK types.
			remoteMetadata = json.RawMessage(initialMetadata)
			importResp := resource.ImportStateResponse{State: stateFor(envVarModel{})}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, &importResp)
			if importResp.Diagnostics.HasError() {
				t.Fatal(importResp.Diagnostics)
			}
			readResp := resource.ReadResponse{State: importResp.State}
			r.Read(ctx, resource.ReadRequest{State: importResp.State}, &readResp)
			if readResp.Diagnostics.HasError() {
				t.Fatal(readResp.Diagnostics)
			}
			model = modelFrom(readResp.State)
			if !scoreJSONEqual(model.Metadata, types.StringValue(initialMetadata)) ||
				!strings.Contains(model.Metadata.ValueString(), "9007199254740993") ||
				!strings.Contains(model.Metadata.ValueString(), "0.1234567890123456789") ||
				!model.ValueVersion.IsNull() {
				t.Fatal("import/read lost metadata precision or invented a value version")
			}
		})
	}
}

func TestEnvMetadataValidation(t *testing.T) {
	ctx := context.Background()
	r := &envVarResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	for _, invalid := range []string{`[]`, `null`, `"secret-must-not-leak"`, `123`, `{"secret-must-not-leak":`} {
		t.Run(invalid, func(t *testing.T) {
			state := tfsdk.State{Schema: schemaResp.Schema}
			if diags := state.Set(ctx, envVarModel{Metadata: types.StringValue(invalid)}); diags.HasError() {
				t.Fatal(diags)
			}
			plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: state.Raw}
			config := tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}
			prior := tfsdk.State{Schema: schemaResp.Schema}
			if diags := prior.Set(ctx, envVarModel{Metadata: types.StringValue(`{"old":true}`)}); diags.HasError() {
				t.Fatal(diags)
			}
			var createResp resource.CreateResponse
			r.Create(ctx, resource.CreateRequest{Plan: plan, Config: config}, &createResp)
			var updateResp resource.UpdateResponse
			r.Update(ctx, resource.UpdateRequest{Plan: plan, Config: config, State: prior}, &updateResp)
			for _, diags := range []string{fmt.Sprint(createResp.Diagnostics), fmt.Sprint(updateResp.Diagnostics)} {
				if !strings.Contains(diags, "metadata must be a JSON object") || strings.Contains(diags, "secret-must-not-leak") {
					t.Fatalf("unexpected validation diagnostics: %s", diags)
				}
			}
		})
	}
}
