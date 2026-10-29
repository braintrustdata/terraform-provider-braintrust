package offline

import (
	"regexp"
	"strings"
)

var scenarios = []scenario{
	{name: "all_without_acls", command: "generate", types: "all", flags: []string{"--org-id", org}, check: checkACLsOmittedByDefault},
	{name: "all_with_acls", command: "generate", types: "all", flags: []string{"--org-id", org, "--include-acls"}, check: func(r *result) {
		r.failIfNot(r.requestsTo("/v1/acl/list_org") > 0 && r.aclReads() > 0 && r.has("braintrust_acl"), "--include-acls did not include ACL discovery and output")
	}},
	{name: "all_import_without_acls", command: "genimport", types: "all", flags: []string{"--org-id", org}, check: checkACLsOmittedByDefault},
	{name: "automation_config", command: "generate", types: "braintrust_project,braintrust_project_automation", check: func(r *result) {
		r.failIfNot(r.has("retention_config"), "automation config omitted")
	}},
	{name: "export_automation_config", command: "generate", types: "braintrust_project,braintrust_project_automation", setup: setupExportAutomation, check: func(r *result) {
		r.failIfNot(r.has("export_config") && r.has("aws_iam_credentials") &&
			r.matches(`external_id\s*=\s*"fixture-external-id"`) &&
			r.matches(`btql_query\s*=\s*"SELECT \* FROM project_logs"`), "required S3 export fields omitted")
	}},
	{name: "unknown_automation_config", command: "generate", types: "braintrust_project,braintrust_project_automation", setup: func(a *fakeAPI) {
		a.fixtures["/v1/project_automation"][0]["config"] = obj{"event_type": "future_event", "new_setting": true}
	}, check: checkRawAutomationConfig},
	{name: "topic_automation_config", command: "generate", types: "braintrust_project,braintrust_project_automation", setup: setupTopicAutomation, check: checkRawAutomationConfig},
	{name: "topic_automation_import", command: "genimport", types: "braintrust_project_automation", plan: true, setup: setupTopicAutomation, check: func(r *result) {
		checkRawAutomationConfig(r)
		r.failIfNot(strings.Contains(r.tfStdout, "Plan: 1 to import, 0 to add, 0 to change, 0 to destroy."), "topic automation import would change the existing config")
	}},
	{name: "function_generation", command: "generate", types: "braintrust_project,braintrust_function", check: func(r *result) {
		r.failIfNot(r.count(`resource "braintrust_function"`) == 3 && r.has("function_data_json") && r.has("prompt_data_json") &&
			r.has("braintrust_project.") && r.has("tags") && r.has("preprocessor") && r.has("scorer"), "prompt, scorer, or preprocessor definition omitted")
	}},
	{name: "function_import", command: "genimport", types: "braintrust_project,braintrust_function", check: func(r *result) {
		r.failIfNot(r.count(`resource "braintrust_function"`) == 3 && r.count("to = braintrust_function.") == 3, "function adoption omitted resources or import blocks")
	}},
	{name: "dataset_definition", command: "generate", types: "braintrust_project,braintrust_dataset", check: func(r *result) {
		r.failIfNot(r.has("metadata_json") && r.has("tags"), "dataset metadata or tags omitted")
	}},
	{name: "invalid_dataset_metadata", command: "generate", types: "braintrust_project,braintrust_dataset", wantFail: true, setup: func(a *fakeAPI) {
		a.fixtures["/v1/dataset"][0]["metadata"] = []string{"not-an-object"}
	}},
	{name: "score_definition", command: "generate", types: "braintrust_project,braintrust_project_score", check: func(r *result) {
		r.failIfNot(r.has("categories_json") && r.has("config_json"), "score definition fields omitted")
	}},
	{name: "view_definition", command: "generate", types: "braintrust_project,braintrust_view", check: func(r *result) {
		r.failIfNot(r.has("custom_charts") && r.has("view_data_json") && r.matches(`object_id\s*=\s*braintrust_project\.`) &&
			r.matches(`object_type\s*=\s*"org_project"`) && r.matches(`"projectId"\s*:\s*braintrust_project\.`), "scoped dashboard or saved view omitted")
	}},
	{name: "view_import", command: "genimport", types: "braintrust_project,braintrust_view", check: func(r *result) {
		r.failIfNot(r.has("org_project/"+org+"/00000000-0000-4000-8000-000000000032"), "scope-aware dashboard import ID omitted")
	}},
	{name: "new_resources", command: "genimport", types: "braintrust_project,braintrust_org_automation,braintrust_mcp_server", check: func(r *result) {
		r.failIfNot(r.has(`resource "braintrust_org_automation"`) && r.has(`resource "braintrust_mcp_server"`) &&
			r.has("retention_days = 0") && r.has("project_id = braintrust_project.") &&
			!r.has("other-org-retention") && !r.has("unselected-mcp") && !r.has("deleted-mcp") &&
			r.count("import {") == 3, "new resources, scope filtering, or imports incorrect")
	}},
	{name: "unknown_org_automation_config", command: "generate", types: "braintrust_org_automation", wantFail: true, setup: func(a *fakeAPI) {
		a.fixtures["/v1/org_automation"][0]["config"].(obj)["new_setting"] = true
	}},
	{name: "multi_project_fetch", command: "generate", types: "braintrust_project,braintrust_dataset,braintrust_function,braintrust_project_score,braintrust_project_tag,braintrust_project_automation", flags: []string{"--org-id", org}, setup: func(a *fakeAPI) {
		a.fixtures["/v1/project"] = append(a.fixtures["/v1/project"],
			obj{"id": secondProject, "org_id": org, "name": "second-project"},
			obj{"id": "90000000-0000-4000-8000-000000000002", "org_id": otherOrg, "name": "other-org-project"})
		a.fixtures["/v1/dataset"] = append(a.fixtures["/v1/dataset"],
			obj{"id": "00000000-0000-4000-8000-000000000056", "project_id": secondProject, "name": "second-dataset"},
			obj{"id": "90000000-0000-4000-8000-000000000056", "project_id": "90000000-0000-4000-8000-000000000002", "name": "other-org-dataset"})
	}, check: func(r *result) {
		for _, path := range []string{"dataset", "function", "project_score", "project_tag", "project_automation"} {
			r.failIfNot(r.requestsTo("/v1/"+path) == 1, path+" was not listed exactly once")
		}
		r.failIfNot(r.count(`resource "braintrust_dataset"`) == 2 && !r.has("other-org-dataset") && r.count(`resource "braintrust_project_automation"`) == 1,
			"selected project resources were duplicated or crossed organizations")
	}},
	{name: "weighted_score_definition", command: "generate", types: "braintrust_project,braintrust_project_score", setup: func(a *fakeAPI) {
		score := a.fixtures["/v1/project_score"][0]
		score["score_type"], score["categories"] = "weighted", obj{"Accuracy": 0.7, "Safety": 0.3}
	}},
	{name: "minimum_score_definition", command: "generate", types: "braintrust_project,braintrust_project_score", setup: func(a *fakeAPI) {
		score := a.fixtures["/v1/project_score"][0]
		score["score_type"], score["categories"] = "minimum", []string{"Accuracy", "Safety"}
	}},
	{name: "invalid_score_definition", command: "generate", types: "braintrust_project,braintrust_project_score", wantFail: true, setup: func(a *fakeAPI) {
		a.fixtures["/v1/project_score"][0]["categories"] = "not-json-structure"
	}},
	{name: "builtin_role_reference", command: "generate", types: "braintrust_project,braintrust_role,braintrust_acl", check: func(r *result) {
		r.failIfNot(r.matches(`role_id\s*=\s*data\.braintrust_role\.`), "built-in role has no data source reference")
		r.failIfNot(r.matches(`object_type\s*=\s*"prompt"`), "org-wide prompt ACL omitted")
	}},
	{name: "builtin_group_membership", command: "generate", types: "braintrust_group,braintrust_group_membership,braintrust_acl", plan: true, setup: setupBuiltinGroups, check: func(r *result) {
		checkBuiltinGroups(r)
		r.failIfNot(r.matches(`group_id\s*=\s*data\.braintrust_group\.`) && r.matches(`user_id\s*=\s*data\.braintrust_user\.`), "built-in membership lacks stable data source references")
	}},
	{name: "builtin_group_import", command: "genimport", types: "braintrust_group,braintrust_group_membership,braintrust_acl", setup: setupBuiltinGroups, check: func(r *result) {
		checkBuiltinGroups(r)
		r.failIfNot(r.count("import {") == 6, "expected custom group, two human memberships, and three ACL imports")
	}},
	{name: "builtin_group_no_links", command: "generate", types: "braintrust_group,braintrust_group_membership", flags: []string{"--no-linked-resources"}, setup: setupBuiltinGroups, check: func(r *result) {
		checkBuiltinGroups(r)
		r.failIf(r.matches(`group_id\s*=\s*data\.braintrust_group\.`) || r.matches(`user_id\s*=\s*data\.braintrust_user\.`), "raw-ID mode unexpectedly linked group membership")
	}},
	{name: "builtin_group_strict", command: "generate", types: "braintrust_group,braintrust_group_membership", flags: []string{"--strict-user-linking"}, setup: setupBuiltinGroups, check: checkBuiltinGroups},
	{name: "raw_acl_user", command: "generate", types: "braintrust_project,braintrust_acl", flags: []string{"--no-linked-resources"}, check: func(r *result) {
		r.failIfNot(r.matches(`user_id\s*=\s*"`+regexp.QuoteMeta(user)+`"`), "raw ACL user ID omitted")
	}},
	{name: "subset_project_reference", command: "generate", types: "braintrust_dataset", check: func(r *result) {
		r.failIf(r.has("braintrust_project."), "references a project absent from the selected resource types")
	}},
	{name: "project_group_links", command: "generate", types: "braintrust_project,braintrust_project_group,braintrust_acl", flags: []string{"--org-id", org}, setup: setupProjectGroups, check: func(r *result) {
		r.failIfNot(r.has("braintrust_project_group") && r.has("member_projects") && r.has("braintrust_project.") &&
			r.matches(`object_id\s*=\s*braintrust_project_group\.`), "project membership or ACL target references omitted")
	}},
	{name: "project_group_membership_links", command: "generate", types: "braintrust_project,braintrust_project_group,braintrust_project_group_membership,braintrust_acl", flags: []string{"--org-id", org}, setup: setupProjectGroups, check: func(r *result) {
		r.failIfNot(r.has("braintrust_project_group_membership") && !r.has("member_projects") &&
			r.matches(`project_id\s*=\s*braintrust_project\.`) && r.matches(`project_group_id\s*=\s*braintrust_project_group\.`), "separate memberships or graph references missing")
	}},
	{name: "project_group_subset", command: "generate", types: "braintrust_project,braintrust_project_group", flags: []string{"--org-id", org, "--projects", "audit-project"}, setup: setupProjectGroups, check: func(r *result) {
		r.failIfNot(r.has(secondProject) && r.has("braintrust_project."), "omitted project should remain a raw ID while emitted project is linked")
	}},
	{name: "project_group_membership_subset", command: "generate", types: "braintrust_project,braintrust_project_group,braintrust_project_group_membership", flags: []string{"--org-id", org, "--projects", "audit-project"}, setup: setupProjectGroups, check: func(r *result) {
		r.failIfNot(r.matches(`project_id\s*=\s*"`+regexp.QuoteMeta(secondProject)+`"`) && r.matches(`project_id\s*=\s*braintrust_project\.`) && !r.has("member_projects"),
			"unselected project membership should retain a raw ID without double management")
	}},
	{name: "project_group_import", command: "genimport", types: "braintrust_project_group", flags: []string{"--org-id", org}, plan: true, setup: setupProjectGroups, check: func(r *result) {
		r.failIfNot(r.has("braintrust_project_group") && r.has(projectGroup), "project group import block omitted")
	}},
	{name: "project_group_membership_import", command: "genimport", types: "braintrust_project_group,braintrust_project_group_membership", flags: []string{"--org-id", org}, plan: true, setup: setupProjectGroups, check: func(r *result) {
		r.failIfNot(r.has("braintrust_project_group_membership") && r.has(projectGroup+"/"+project) && !r.has("member_projects"), "membership adoption is incomplete or double-managed")
	}},
	{name: "service_token_import", command: "genimport", types: "braintrust_service_token", plan: true, check: func(r *result) {
		r.failIf(r.has(`resource "`) || r.has("import {") || !r.reported("omitted 1"), "unsupported import was not explicitly omitted")
	}},
	{name: "denied_acl_discovery", command: "generate", types: "braintrust_project,braintrust_acl", flags: []string{"--no-linked-resources"}, wantFail: true, setup: func(a *fakeAPI) {
		a.denyACLs = true
	}},
	{name: "secret_variable_syntax", command: "generate", types: "braintrust_ai_secret", check: func(r *result) {
		r.failIfNot(r.matches(`type\s*=\s*string`) && r.matches(`sensitive\s*=\s*true`) && r.matches(`secret\s*=\s*var\.braintrust_ai_secret_`), "AI secret has no required sensitive variable reference")
		r.failIfNot(r.reported("required sensitive variable(s)"), "required secret variables were not reported")
	}},
	{name: "secret_variable_collision", command: "generate", types: "braintrust_ai_secret", setup: func(a *fakeAPI) {
		a.fixtures["/v1/ai_secret"] = []obj{
			{"id": "10000000-0000-4000-8000-000000000010", "name": "Foo Bar", "type": "openai", "org_id": org},
			{"id": "20000000-0000-4000-8000-000000000010", "name": "Foo-Bar", "type": "openai", "org_id": org},
		}
	}, check: func(r *result) {
		names := map[string]bool{}
		declarations := regexp.MustCompile(`variable "([^"]+)"`).FindAllStringSubmatch(r.hcl, -1)
		for _, m := range declarations {
			names[m[1]] = true
		}
		r.failIfNot(len(declarations) == 2 && len(names) == 2, "secret variable names collided")
	}},
	{name: "ordinary_env_var_value", command: "generate", types: "braintrust_project,braintrust_env_var", check: func(r *result) {
		r.failIfNot(r.matches(`value\s*=\s*var\.braintrust_env_var_`), "write-only ordinary env var value has no variable")
		r.failIfNot(r.matches(`object_id\s*=\s*braintrust_project\.`), "project env var still points to a source-org project ID")
		r.failIfNot(r.reported("required sensitive variable(s)"), "required secret variables were not reported")
	}},
	{name: "secret_metadata_adoption", command: "genimport", types: "braintrust_project,braintrust_ai_secret,braintrust_env_var", plan: true, setup: func(a *fakeAPI) {
		a.fixtures["/v1/ai_secret"][0]["type"] = "azure"
		a.fixtures["/v1/ai_secret"][0]["metadata"] = obj{"api_base": "https://example.openai.azure.com", "deployment": "org-deployment", "future": obj{"enabled": true}}
		a.fixtures["/v1/env_var"][0]["secret_type"] = "azure"
		a.fixtures["/v1/env_var"][0]["metadata"] = obj{"api_base": "https://example.openai.azure.com", "deployment": "project-deployment", "future": obj{"models": []string{"custom-model"}}}
	}, check: func(r *result) {
		r.failIfNot(r.matches(`metadata\s*=`) && r.has("org-deployment") && r.has("project-deployment") && r.has("custom-model"), "AI provider metadata was omitted from exported configuration")
	}},
	{name: "ai_secret_adoption", command: "genimport", types: "braintrust_ai_secret", check: func(r *result) {
		r.failIf(r.has(`variable "`) || r.matches(`secret\s*=\s*var\.`), "import configuration would rotate an unknown write-only value")
	}},
	{name: "stale_acl_unverified", command: "generate", types: "braintrust_project,braintrust_acl", flags: []string{"--verify-acl-targets=false"}, setup: setupStaleACL, check: func(r *result) {
		r.failIfNot(r.has(`resource "braintrust_acl" "terraform_managed_resource_50000000"`) && r.reported("ACL target(s) were not verified"), "unverified ACL was omitted or warning is missing")
	}},
	{name: "stale_acl_omission", command: "generate", types: "braintrust_project,braintrust_acl", setup: setupStaleACL, check: func(r *result) {
		r.failIf(r.has(`resource "braintrust_acl" "terraform_managed_resource_50000000"`) || !r.reported("skipped 1 ACL"), "confirmed dead ACL was exported or omitted silently")
		r.failIfNot(r.aclReads() > 0, "ACL verification made no individual reads")
	}},
	{name: "explicit_org_scope", command: "generate", types: "braintrust_project", flags: []string{"--org-id", org}},
	{name: "wrong_org_scope", command: "generate", types: "braintrust_project", flags: []string{"--org-id", otherOrg}, wantFail: true},
	{name: "multi_org_selected", command: "generate", types: "braintrust_project,braintrust_role,braintrust_group,braintrust_ai_secret", flags: []string{"--org-id", org}, setup: func(a *fakeAPI) {
		addOtherOrgProject(a)
		a.fixtures["/v1/role"] = append(a.fixtures["/v1/role"], obj{"id": "90000000-0000-4000-8000-000000000003", "org_id": otherOrg, "name": "other-org-role"})
		a.fixtures["/v1/group"] = []obj{{"id": "90000000-0000-4000-8000-000000000004", "org_id": otherOrg, "name": "other-org-group"}}
		a.fixtures["/v1/ai_secret"] = append(a.fixtures["/v1/ai_secret"], obj{"id": "90000000-0000-4000-8000-000000000010", "org_id": otherOrg, "name": "other-org-secret", "type": "openai"})
	}, check: func(r *result) {
		r.failIf(r.has("other-org-"), "emitted resource from another org")
	}},
	{name: "multi_org_ambiguous", command: "generate", types: "braintrust_project", wantFail: true, setup: addOtherOrgProject},
	{name: "empty_org_env_var", command: "generate", types: "braintrust_env_var", flags: []string{"--org-id", org}, setup: setupEmptyOrg, check: func(r *result) {
		r.failIfNot(r.has("ORG_VALUE"), "omitted org env var when org has no projects")
	}},
	{name: "empty_org_unscoped", command: "generate", types: "braintrust_env_var", wantFail: true, setup: setupEmptyOrg},
	{name: "unknown_project_selector", command: "generate", types: "braintrust_dataset", flags: []string{"--projects", "missing-project"}, wantFail: true},
	{name: "mismatched_acl_scope", command: "generate", types: "braintrust_acl", flags: []string{"--org-id", org}, wantFail: true, setup: func(a *fakeAPI) {
		a.fixtures["/v1/acl/list_org"][0]["_object_org_id"] = otherOrg
	}},
}

func checkACLsOmittedByDefault(r *result) {
	r.failIf(r.requestsTo("/v1/acl/list_org") > 0 || r.aclReads() > 0 || r.has("braintrust_acl"), "default all unexpectedly included ACL discovery or output")
	r.failIfNot(r.reported("all excludes braintrust_acl"), "default ACL omission was not reported")
}

func checkRawAutomationConfig(r *result) {
	r.failIfNot(r.has("config_json = ") && !r.has("logs_config = "), "raw automation config omitted or misclassified")
}

func checkBuiltinGroups(r *result) {
	r.failIf(r.matches(`resource "braintrust_group" "[^"]+"\s*\{[^}]*name\s*=\s*"(?:All AI Provider Access|Engineers)"`), "built-in group was declared as managed")
	r.failIfNot(r.count(`resource "braintrust_group_membership"`) == 2, "human memberships in built-in groups were not captured")
	r.failIf(r.has(serviceUser) || r.has("45000000-0000-4000-8000-000000000045"), "service membership or default ACL became managed")
	r.failIfNot(r.reported("skipped 1 distinct built-in group member"), "skipped service membership was not reported")
}

func setupExportAutomation(a *fakeAPI) {
	a.fixtures["/v1/project_automation"] = []obj{{
		"id":         "70000000-0000-4000-8000-000000000017",
		"project_id": project,
		"name":       "audit-export",
		"config": obj{
			"event_type":        "btql_export",
			"export_path":       "s3://fixture-bucket/audit/",
			"format":            "jsonl",
			"interval_seconds":  2592000,
			"export_definition": obj{"type": "btql_query", "btql_query": "SELECT * FROM project_logs"},
			"credentials":       obj{"role_arn": "arn:aws:iam::123456789012:role/fixture", "external_id": "fixture-external-id"},
		},
	}}
}

func setupTopicAutomation(a *fakeAPI) {
	a.fixtures["/v1/project_automation"][0]["config"] = obj{"event_type": "topic", "sampling_rate": 0.5, "facet_functions": []any{}, "topic_map_functions": []any{}, "btql_filter": "metadata.env = 'prod'"}
}

func setupProjectGroups(a *fakeAPI) {
	a.fixtures["/v1/project"] = append(a.fixtures["/v1/project"], obj{"id": secondProject, "org_id": org, "name": "second-project"})
	a.fixtures["/v1/project_group"] = []obj{{"id": projectGroup, "org_id": org, "name": "audit-group", "member_projects": []string{project, secondProject}}}
	a.fixtures["/v1/acl/list_org"] = append(a.fixtures["/v1/acl/list_org"], obj{"id": "70000000-0000-4000-8000-000000000024", "object_type": "project_group", "object_id": projectGroup, "user_id": user, "permission": "read", "_object_org_id": org})
}

func setupBuiltinGroups(a *fakeAPI) {
	a.fixtures["/v1/group"] = []obj{
		{"id": builtinGroup, "org_id": org, "name": "All AI Provider Access", "member_users": []string{user, serviceUser}},
		{"id": engineers, "org_id": org, "name": "Engineers", "member_users": []string{user}},
		{"id": customGroup, "org_id": org, "name": "Custom Reviewers", "member_users": []string{user}},
	}
	a.fixtures["/v1/acl/list_org"] = append(a.fixtures["/v1/acl/list_org"],
		obj{"id": "45000000-0000-4000-8000-000000000045", "object_type": "org_ai_secret", "object_id": org, "group_id": builtinGroup, "permission": "read", "_object_org_id": org},
		obj{"id": "46000000-0000-4000-8000-000000000046", "object_type": "project", "object_id": project, "group_id": builtinGroup, "permission": "read", "_object_org_id": org})
}

func setupStaleACL(a *fakeAPI) {
	a.staleACLID = staleACL
	a.fixtures["/v1/acl/list_org"] = append(a.fixtures["/v1/acl/list_org"], obj{"id": staleACL, "object_type": "project", "object_id": "60000000-0000-4000-8000-000000000015", "user_id": user, "permission": "read", "_object_org_id": org})
}

func setupEmptyOrg(a *fakeAPI) {
	a.fixtures["/v1/project"] = []obj{}
	a.fixtures["/v1/env_var"] = []obj{{"id": "90000000-0000-4000-8000-000000000011", "name": "ORG_VALUE", "object_type": "organization", "object_id": org}}
}

func addOtherOrgProject(a *fakeAPI) {
	a.fixtures["/v1/project"] = append(a.fixtures["/v1/project"], obj{"id": "90000000-0000-4000-8000-000000000002", "org_id": otherOrg, "name": "other-org-project"})
}
