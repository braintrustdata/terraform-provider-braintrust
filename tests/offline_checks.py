#!/usr/bin/env python3
"""Reproduce exporter/provider integration defects without a Braintrust credential.

Build both binaries first, then pass --provider and --exporter. All API calls go
to a local fixture server. Terraform uses a temporary development override.
Use --skip-terraform to check exporter output without access to the provider.
"""

import argparse
import copy
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


ORG = "00000000-0000-4000-8000-000000000001"
OTHER_ORG = "90000000-0000-4000-8000-000000000001"
PROJECT = "00000000-0000-4000-8000-000000000002"
PROJECT_GROUP = "00000000-0000-4000-8000-000000000022"
SECOND_PROJECT = "00000000-0000-4000-8000-000000000023"
ROLE = "00000000-0000-4000-8000-000000000003"
USER = "00000000-0000-4000-8000-000000000004"
BUILTIN_GROUP = "41000000-0000-4000-8000-000000000041"
ENGINEERS_GROUP = "42000000-0000-4000-8000-000000000042"
CUSTOM_GROUP = "43000000-0000-4000-8000-000000000043"
SERVICE_USER = "44000000-0000-4000-8000-000000000044"
FIXTURES = {
    "/v1/organization": [{"id": ORG}],
    "/v1/project": [{"id": PROJECT, "org_id": ORG, "name": "audit-project"}],
    "/v1/project_group": [],
    "/v1/role": [{"id": ROLE, "name": "Viewer", "org_id": None}],
    "/v1/user": [{"id": USER, "email": "fixture@example.invalid"}],
    "/v1/group": [],
    "/v1/acl": [{"id": "00000000-0000-4000-8000-000000000005", "object_type": "project", "object_id": PROJECT, "user_id": USER, "role_id": ROLE, "_object_org_id": ORG}],
    "/v1/acl/list_org": [
        {"id": "00000000-0000-4000-8000-000000000005", "object_type": "project", "object_id": PROJECT, "user_id": USER, "role_id": ROLE, "_object_org_id": ORG},
        {"id": "30000000-0000-4000-8000-000000000012", "object_type": "prompt", "object_id": "40000000-0000-4000-8000-000000000013", "user_id": USER, "permission": "read", "_object_org_id": ORG},
    ],
    "/v1/dataset": [{"id": "00000000-0000-4000-8000-000000000006", "project_id": PROJECT, "name": "audit-dataset", "tags": ["reviewed", "gold"], "metadata": {"owner": "qa", "nested": {"tier": 1}}}],
    "/v1/project_score": [{"id": "90000000-0000-4000-8000-000000000006", "project_id": PROJECT, "name": "Audit Quality", "score_type": "categorical", "categories": [{"name": "Low", "value": 0}, {"name": "High", "value": 1}], "config": {"object_types": ["project_logs"], "multi_select": False}}],
    "/v1/function": [
        {"id": "00000000-0000-4000-8000-000000000016", "project_id": PROJECT, "name": "Audit Prompt", "slug": "audit-prompt", "tags": ["audit"], "function_data": {"type": "prompt"}, "prompt_data": {"prompt": "hello"}},
        {"id": "00000000-0000-4000-8000-000000000066", "project_id": PROJECT, "name": "Audit Scorer", "slug": "audit-scorer", "function_type": "scorer", "function_data": {"type": "code", "data": {"type": "inline", "runtime_context": {"runtime": "quickjs", "version": "ES2023"}, "code": "function main() { return 1; }"}}},
        {"id": "00000000-0000-4000-8000-000000000067", "project_id": PROJECT, "name": "Audit Preprocessor", "slug": "audit-preprocessor", "function_type": "preprocessor", "function_data": {"type": "global", "name": "thread", "function_type": "preprocessor"}},
    ],
    "/v1/project_automation": [{"id": "00000000-0000-4000-8000-000000000007", "project_id": PROJECT, "name": "audit-retention", "config": {"event_type": "retention", "object_type": "project_logs", "retention_days": 90}}],
    "/v1/org_automation": [
        {"id": "00000000-0000-4000-8000-000000000051", "org_id": ORG, "name": "org-retention", "config": {"event_type": "retention", "object_type": "dataset", "retention_days": 0}},
        {"id": "00000000-0000-4000-8000-000000000052", "org_id": OTHER_ORG, "name": "other-org-retention", "config": {"event_type": "retention", "object_type": "dataset", "retention_days": 30}},
    ],
    "/v1/mcp_server": [
        {"id": "00000000-0000-4000-8000-000000000053", "project_id": PROJECT, "name": "audit-mcp", "url": "https://mcp.example.invalid", "deleted_at": None},
        {"id": "00000000-0000-4000-8000-000000000054", "project_id": SECOND_PROJECT, "name": "unselected-mcp", "url": "https://other.example.invalid"},
        {"id": "00000000-0000-4000-8000-000000000055", "project_id": PROJECT, "name": "deleted-mcp", "url": "https://deleted.example.invalid", "deleted_at": "2026-01-01T00:00:00Z"},
    ],
    "/v1/service_token": [{"id": "00000000-0000-4000-8000-000000000008", "name": "audit-token", "service_account_id": USER, "org_id": ORG}],
    "/v1/ai_secret": [{"id": "00000000-0000-4000-8000-000000000010", "name": "OpenAI", "type": "openai", "org_id": ORG}],
    "/v1/env_var": [{"id": "00000000-0000-4000-8000-000000000011", "name": "ORDINARY_VALUE", "object_type": "project", "object_id": PROJECT}],
    "/v1/view": [
        {"id": "00000000-0000-4000-8000-000000000031", "object_type": "project", "object_id": PROJECT, "view_type": "logs", "name": "Errors", "view_data": {"search": {"filter": []}}},
        {"id": "00000000-0000-4000-8000-000000000032", "object_type": "org_project", "object_id": ORG, "view_type": "monitor", "name": "Health", "options": {"viewType": "monitor", "options": {"projectId": PROJECT, "rangeValue": "7d"}}, "view_data": {"custom_charts": {"version": "0.0.0", "layout": {"type": "linear", "order": ["note"]}, "charts": {"note": {"type": "note", "markdown": "# Health"}}}}},
    ],
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--provider", type=Path)
    parser.add_argument("--exporter", type=Path, required=True)
    parser.add_argument("--skip-terraform", action="store_true")
    parser.add_argument("--output", type=Path, default=Path(__file__).parent / "evidence" / "offline")
    args = parser.parse_args()
    if not args.skip_terraform and args.provider is None:
        parser.error("--provider is required unless --skip-terraform is set")
    provider = args.provider.resolve() if args.provider else None
    exporter = args.exporter.resolve()
    args.output.mkdir(parents=True, exist_ok=True)
    requests = []
    state = {"deny_acls": False, "stale_acl_id": None, "fixtures": FIXTURES}

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            requests.append(self.path)
            path = urlparse(self.path).path
            if state["deny_acls"] and path == "/v1/acl/list_org":
                self.send_response(403)
                body = {"error": "fixture: missing read_acls permission"}
            elif path.startswith("/v1/acl/") and path != "/v1/acl/list_org":
                acl_id = path.rsplit("/", 1)[-1]
                if acl_id == state["stale_acl_id"]:
                    self.send_response(400)
                    body = {"error": "ACL does not exist"}
                else:
                    matched = next((acl for acl in state["fixtures"]["/v1/acl/list_org"] if acl["id"] == acl_id), None)
                    self.send_response(200 if matched else 404)
                    body = matched or {"error": "ACL does not exist"}
            elif path.startswith("/v1/project_group/"):
                group_id = path.rsplit("/", 1)[-1]
                matched = next((group for group in state["fixtures"]["/v1/project_group"] if group["id"] == group_id), None)
                self.send_response(200 if matched else 404)
                body = matched or {"error": "Project group does not exist"}
            elif path.startswith("/v1/project_automation/"):
                automation_id = path.rsplit("/", 1)[-1]
                matched = next((automation for automation in state["fixtures"]["/v1/project_automation"] if automation["id"] == automation_id), None)
                self.send_response(200 if matched else 404)
                body = matched or {"error": "Project automation does not exist"}
            elif path.startswith("/v1/group/"):
                group_id = path.rsplit("/", 1)[-1]
                matched = next((group for group in state["fixtures"]["/v1/group"] if group["id"] == group_id), None)
                self.send_response(200 if matched else 404)
                body = matched or {"error": "Group does not exist"}
            else:
                self.send_response(200)
                body = {"objects": state["fixtures"].get(path, [])}
                if path == "/v1/view":
                    query = parse_qs(urlparse(self.path).query)
                    body = {"objects": [v for v in state["fixtures"][path] if v["object_type"] == query.get("object_type", [""])[0] and v["object_id"] == query.get("object_id", [""])[0]]}
                if "starting_after=" in self.path:
                    body = {"objects": []}
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(body).encode())

        def do_PATCH(self):
            path = urlparse(self.path).path
            group_id = path.rsplit("/", 1)[-1]
            group = next((item for item in state["fixtures"]["/v1/group"] if item["id"] == group_id), None) if path.startswith("/v1/group/") else None
            if group is None:
                self.send_response(404)
                body = {"error": "Group does not exist"}
            else:
                patch = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                members = set(group.get("member_users", []))
                members.update(patch.get("add_member_users", []))
                members.difference_update(patch.get("remove_member_users", []))
                group["member_users"] = sorted(members)
                self.send_response(200)
                body = group
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(body).encode())

        def log_message(self, *unused):
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    api_url = f"http://127.0.0.1:{server.server_port}"
    results = []
    failures = []
    scenarios = [
        ("all_without_acls", "generate", "all", ["--org-id", ORG], "validate"),
        ("all_with_acls", "generate", "all", ["--org-id", ORG, "--include-acls"], "validate"),
        ("all_import_without_acls", "genimport", "all", ["--org-id", ORG], "validate"),
        ("automation_config", "generate", "braintrust_project,braintrust_project_automation", [], "validate"),
        ("export_automation_config", "generate", "braintrust_project,braintrust_project_automation", [], "validate"),
        ("unknown_automation_config", "generate", "braintrust_project,braintrust_project_automation", [], "validate"),
        ("topic_automation_config", "generate", "braintrust_project,braintrust_project_automation", [], "validate"),
        ("topic_automation_import", "genimport", "braintrust_project_automation", [], "plan"),
        ("function_generation", "generate", "braintrust_project,braintrust_function", [], "validate"),
        ("function_import", "genimport", "braintrust_project,braintrust_function", [], "validate"),
        ("dataset_definition", "generate", "braintrust_project,braintrust_dataset", [], "validate"),
        ("invalid_dataset_metadata", "generate", "braintrust_project,braintrust_dataset", [], "validate"),
        ("score_definition", "generate", "braintrust_project,braintrust_project_score", [], "validate"),
        ("view_definition", "generate", "braintrust_project,braintrust_view", [], "validate"),
        ("view_import", "genimport", "braintrust_project,braintrust_view", [], "validate"),
        ("new_resources", "genimport", "braintrust_project,braintrust_org_automation,braintrust_mcp_server", [], "validate"),
        ("unknown_org_automation_config", "generate", "braintrust_org_automation", [], "validate"),
        ("multi_project_fetch", "generate", "braintrust_project,braintrust_dataset,braintrust_function,braintrust_project_score,braintrust_project_tag,braintrust_project_automation", ["--org-id", ORG], "validate"),
        ("weighted_score_definition", "generate", "braintrust_project,braintrust_project_score", [], "validate"),
        ("minimum_score_definition", "generate", "braintrust_project,braintrust_project_score", [], "validate"),
        ("invalid_score_definition", "generate", "braintrust_project,braintrust_project_score", [], "validate"),
        ("builtin_role_reference", "generate", "braintrust_project,braintrust_role,braintrust_acl", [], "validate"),
        ("builtin_group_membership", "generate", "braintrust_group,braintrust_group_membership,braintrust_acl", [], "plan"),
        ("builtin_group_import", "genimport", "braintrust_group,braintrust_group_membership,braintrust_acl", [], "validate"),
        ("builtin_group_no_links", "generate", "braintrust_group,braintrust_group_membership", ["--no-linked-resources"], "validate"),
        ("builtin_group_strict", "generate", "braintrust_group,braintrust_group_membership", ["--strict-user-linking"], "validate"),
        ("raw_acl_user", "generate", "braintrust_project,braintrust_acl", ["--no-linked-resources"], "validate"),
        ("subset_project_reference", "generate", "braintrust_dataset", [], "validate"),
        ("project_group_links", "generate", "braintrust_project,braintrust_project_group,braintrust_acl", ["--org-id", ORG], "validate"),
        ("project_group_membership_links", "generate", "braintrust_project,braintrust_project_group,braintrust_project_group_membership,braintrust_acl", ["--org-id", ORG], "validate"),
        ("project_group_subset", "generate", "braintrust_project,braintrust_project_group", ["--org-id", ORG, "--projects", "audit-project"], "validate"),
        ("project_group_membership_subset", "generate", "braintrust_project,braintrust_project_group,braintrust_project_group_membership", ["--org-id", ORG, "--projects", "audit-project"], "validate"),
        ("project_group_import", "genimport", "braintrust_project_group", ["--org-id", ORG], "plan"),
        ("project_group_membership_import", "genimport", "braintrust_project_group,braintrust_project_group_membership", ["--org-id", ORG], "plan"),
        ("service_token_import", "genimport", "braintrust_service_token", [], "plan"),
        ("denied_acl_discovery", "generate", "braintrust_project,braintrust_acl", ["--no-linked-resources"], "validate"),
        ("secret_variable_syntax", "generate", "braintrust_ai_secret", [], "validate"),
        ("secret_variable_collision", "generate", "braintrust_ai_secret", [], "validate"),
        ("ordinary_env_var_value", "generate", "braintrust_project,braintrust_env_var", [], "validate"),
        ("ai_secret_adoption", "genimport", "braintrust_ai_secret", [], "validate"),
        ("stale_acl_unverified", "generate", "braintrust_project,braintrust_acl", ["--verify-acl-targets=false"], "validate"),
        ("stale_acl_omission", "generate", "braintrust_project,braintrust_acl", [], "validate"),
        ("explicit_org_scope", "generate", "braintrust_project", ["--org-id", ORG], "validate"),
        ("wrong_org_scope", "generate", "braintrust_project", ["--org-id", OTHER_ORG], "validate"),
        ("multi_org_selected", "generate", "braintrust_project,braintrust_role,braintrust_group,braintrust_ai_secret", ["--org-id", ORG], "validate"),
        ("multi_org_ambiguous", "generate", "braintrust_project", [], "validate"),
        ("empty_org_env_var", "generate", "braintrust_env_var", ["--org-id", ORG], "validate"),
        ("empty_org_unscoped", "generate", "braintrust_env_var", [], "validate"),
        ("unknown_project_selector", "generate", "braintrust_dataset", ["--projects", "missing-project"], "validate"),
        ("mismatched_acl_scope", "generate", "braintrust_acl", ["--org-id", ORG], "validate"),
    ]
    try:
        with tempfile.TemporaryDirectory(prefix="bt-terraform-offline-") as temporary:
            root = Path(temporary)
            binaries = root / "bin"
            binaries.mkdir()
            if provider:
                shutil.copy2(provider, binaries / "terraform-provider-braintrust")
            config = root / "terraform.rc"
            if provider:
                config.write_text('provider_installation {\n  dev_overrides {\n    "braintrustdata/braintrust" = "' + str(binaries) + '"\n  }\n  direct {}\n}\n')
            env = dict(os.environ)
            env.update(BRAINTRUST_API_KEY="offline-fixture-token", BRAINTRUST_API_URL=api_url, TF_IN_AUTOMATION="1", CHECKPOINT_DISABLE="1")
            if provider:
                env["TF_CLI_CONFIG_FILE"] = str(config)
            env.pop("TF_ACC", None)
            env.pop("TF_LOG", None)
            env.pop("TF_LOG_PATH", None)
            for name, command, types, flags, operation in scenarios:
                state["deny_acls"] = name == "denied_acl_discovery"
                state["stale_acl_id"] = "50000000-0000-4000-8000-000000000014" if name in {"stale_acl_unverified", "stale_acl_omission"} else None
                state["fixtures"] = copy.deepcopy(FIXTURES)
                if name.startswith("project_group_"):
                    state["fixtures"]["/v1/project"].append({"id": SECOND_PROJECT, "org_id": ORG, "name": "second-project"})
                    state["fixtures"]["/v1/project_group"] = [{"id": PROJECT_GROUP, "org_id": ORG, "name": "audit-group", "member_projects": [PROJECT, SECOND_PROJECT]}]
                    state["fixtures"]["/v1/acl/list_org"].append({"id": "70000000-0000-4000-8000-000000000024", "object_type": "project_group", "object_id": PROJECT_GROUP, "user_id": USER, "permission": "read", "_object_org_id": ORG})
                if state["stale_acl_id"]:
                    state["fixtures"]["/v1/acl/list_org"].append({"id": state["stale_acl_id"], "object_type": "project", "object_id": "60000000-0000-4000-8000-000000000015", "user_id": USER, "permission": "read", "_object_org_id": ORG})
                if name in {"multi_org_selected", "multi_org_ambiguous"}:
                    state["fixtures"]["/v1/project"].append({"id": "90000000-0000-4000-8000-000000000002", "org_id": OTHER_ORG, "name": "other-org-project"})
                if name == "multi_project_fetch":
                    state["fixtures"]["/v1/project"].extend([
                        {"id": SECOND_PROJECT, "org_id": ORG, "name": "second-project"},
                        {"id": "90000000-0000-4000-8000-000000000002", "org_id": OTHER_ORG, "name": "other-org-project"},
                    ])
                    state["fixtures"]["/v1/dataset"].extend([
                        {"id": "00000000-0000-4000-8000-000000000056", "project_id": SECOND_PROJECT, "name": "second-dataset"},
                        {"id": "90000000-0000-4000-8000-000000000056", "project_id": "90000000-0000-4000-8000-000000000002", "name": "other-org-dataset"},
                    ])
                if name == "multi_org_selected":
                    state["fixtures"]["/v1/role"].append({"id": "90000000-0000-4000-8000-000000000003", "org_id": OTHER_ORG, "name": "other-org-role"})
                    state["fixtures"]["/v1/group"] = [{"id": "90000000-0000-4000-8000-000000000004", "org_id": OTHER_ORG, "name": "other-org-group"}]
                    state["fixtures"]["/v1/ai_secret"].append({"id": "90000000-0000-4000-8000-000000000010", "org_id": OTHER_ORG, "name": "other-org-secret", "type": "openai"})
                if name.startswith("builtin_group_"):
                    state["fixtures"]["/v1/group"] = [
                        {"id": BUILTIN_GROUP, "org_id": ORG, "name": "All AI Provider Access", "member_users": [USER, SERVICE_USER]},
                        {"id": ENGINEERS_GROUP, "org_id": ORG, "name": "Engineers", "member_users": [USER]},
                        {"id": CUSTOM_GROUP, "org_id": ORG, "name": "Custom Reviewers", "member_users": [USER]},
                    ]
                    state["fixtures"]["/v1/acl/list_org"].extend([
                        {"id": "45000000-0000-4000-8000-000000000045", "object_type": "org_ai_secret", "object_id": ORG, "group_id": BUILTIN_GROUP, "permission": "read", "_object_org_id": ORG},
                        {"id": "46000000-0000-4000-8000-000000000046", "object_type": "project", "object_id": PROJECT, "group_id": BUILTIN_GROUP, "permission": "read", "_object_org_id": ORG},
                    ])
                if name in {"empty_org_env_var", "empty_org_unscoped"}:
                    state["fixtures"]["/v1/project"] = []
                    state["fixtures"]["/v1/env_var"] = [{"id": "90000000-0000-4000-8000-000000000011", "name": "ORG_VALUE", "object_type": "organization", "object_id": ORG}]
                if name == "mismatched_acl_scope":
                    state["fixtures"]["/v1/acl/list_org"][0]["_object_org_id"] = OTHER_ORG
                if name == "invalid_score_definition":
                    state["fixtures"]["/v1/project_score"][0]["categories"] = "not-json-structure"
                if name == "invalid_dataset_metadata":
                    state["fixtures"]["/v1/dataset"][0]["metadata"] = ["not-an-object"]
                if name == "weighted_score_definition":
                    state["fixtures"]["/v1/project_score"][0].update(score_type="weighted", categories={"Accuracy": 0.7, "Safety": 0.3})
                if name == "minimum_score_definition":
                    state["fixtures"]["/v1/project_score"][0].update(score_type="minimum", categories=["Accuracy", "Safety"])
                if name == "secret_variable_collision":
                    state["fixtures"]["/v1/ai_secret"] = [
                        {"id": "10000000-0000-4000-8000-000000000010", "name": "Foo Bar", "type": "openai", "org_id": ORG},
                        {"id": "20000000-0000-4000-8000-000000000010", "name": "Foo-Bar", "type": "openai", "org_id": ORG},
                    ]
                if name == "export_automation_config":
                    state["fixtures"]["/v1/project_automation"] = [{
                        "id": "70000000-0000-4000-8000-000000000017",
                        "project_id": PROJECT,
                        "name": "audit-export",
                        "config": {
                            "event_type": "btql_export",
                            "export_path": "s3://fixture-bucket/audit/",
                            "format": "jsonl",
                            "interval_seconds": 2592000,
                            "export_definition": {"type": "btql_query", "btql_query": "SELECT * FROM project_logs"},
                            "credentials": {"role_arn": "arn:aws:iam::123456789012:role/fixture", "external_id": "fixture-external-id"},
                        },
                    }]
                if name == "unknown_automation_config":
                    state["fixtures"]["/v1/project_automation"][0]["config"] = {"event_type": "future_event", "new_setting": True}
                if name in {"topic_automation_config", "topic_automation_import"}:
                    state["fixtures"]["/v1/project_automation"][0]["config"] = {"event_type": "topic", "sampling_rate": 0.5, "facet_functions": [], "topic_map_functions": [], "btql_filter": "metadata.env = 'prod'"}
                if name == "unknown_org_automation_config":
                    state["fixtures"]["/v1/org_automation"][0]["config"]["new_setting"] = True
                workspace = root / name
                workspace.mkdir()
                destination = args.output / name
                destination.mkdir(exist_ok=True)
                requests.clear()
                generated = subprocess.run([str(exporter), command, "--resource-types", types, *flags], env=env, capture_output=True, text=True, timeout=60)
                (workspace / "generated.tf").write_text(generated.stdout)
                (destination / "generated.tf").write_text(generated.stdout)
                (destination / "exporter.stderr.txt").write_text(generated.stderr)
                (workspace / "provider.tf").write_text('terraform {\n required_providers {\n  braintrust = { source = "braintrustdata/braintrust" }\n }\n}\nprovider "braintrust" {}\n')
                checked = None
                if not args.skip_terraform:
                    if operation == "validate":
                        terraform_args = ["terraform", "validate", "-json", "-no-color"]
                    else:
                        terraform_args = ["terraform", "plan", "-input=false", "-refresh=false", "-no-color"]
                    checked = subprocess.run(terraform_args, cwd=workspace, env=env, capture_output=True, text=True, timeout=60)
                    (destination / "terraform.stdout.txt").write_text(checked.stdout)
                    (destination / "terraform.stderr.txt").write_text(checked.stderr)
                result = {"scenario": name, "exporter_exit": generated.returncode, "terraform_exit": checked.returncode if checked else None, "api_requests": list(requests)}
                acl_reads = [request for request in requests if urlparse(request).path.startswith("/v1/acl/") and urlparse(request).path != "/v1/acl/list_org"]
                acl_discovery = [request for request in requests if urlparse(request).path == "/v1/acl/list_org"]
                if name in {"all_without_acls", "all_import_without_acls"} and (acl_discovery or acl_reads or 'braintrust_acl' in generated.stdout):
                    failures.append(f"{name}: default all unexpectedly included ACL discovery or output")
                if name in {"all_without_acls", "all_import_without_acls"} and "all excludes braintrust_acl" not in generated.stderr:
                    failures.append(f"{name}: default ACL omission was not reported")
                if name == "all_with_acls" and (not acl_discovery or not acl_reads or 'braintrust_acl' not in generated.stdout):
                    failures.append(f"{name}: --include-acls did not include ACL discovery and output")
                if "braintrust_acl" in types and "--verify-acl-targets=false" in flags and acl_reads:
                    failures.append(f"{name}: list-only ACL discovery made {len(acl_reads)} individual reads")
                if "braintrust_acl" in types and any(urlparse(request).path == "/v1/acl" for request in requests):
                    failures.append(f"{name}: ACL discovery made a target-scoped request")
                if checked and operation == "validate":
                    diagnostics = json.loads(checked.stdout)
                    result["diagnostics"] = [{"severity": d["severity"], "summary": d["summary"], "detail": d["detail"]} for d in diagnostics.get("diagnostics", [])]
                results.append(result)
                print(name, "exporter", generated.returncode, "terraform", checked.returncode if checked else "skipped")
                if name in {"denied_acl_discovery", "unknown_org_automation_config", "invalid_score_definition", "invalid_dataset_metadata", "wrong_org_scope", "multi_org_ambiguous", "empty_org_unscoped", "unknown_project_selector", "mismatched_acl_scope"}:
                    if generated.returncode == 0 or generated.stdout.strip():
                        failures.append(f"{name}: unsupported discovery must fail without emitting a partial export")
                    continue
                if generated.returncode != 0 or (checked and checked.returncode != 0):
                    failures.append(f"{name}: expected valid generated Terraform")
                    continue
                hcl = generated.stdout
                if name == "automation_config" and "retention_config" not in hcl:
                    failures.append(f"{name}: automation config omitted")
                if name in {"unknown_automation_config", "topic_automation_config", "topic_automation_import"} and ('config_json = ' not in hcl or 'logs_config = ' in hcl):
                    failures.append(f"{name}: raw automation config omitted or misclassified")
                if name == "topic_automation_import" and checked and "Plan: 1 to import, 0 to add, 0 to change, 0 to destroy." not in checked.stdout:
                    failures.append(f"{name}: topic automation import would change the existing config")
                if name == "project_group_links" and ("braintrust_project_group" not in hcl or "member_projects" not in hcl or "braintrust_project." not in hcl or not re.search(r'object_id\s*=\s*braintrust_project_group\.', hcl)):
                    failures.append(f"{name}: project membership or ACL target references omitted")
                if name == "project_group_membership_links" and ("braintrust_project_group_membership" not in hcl or "member_projects" in hcl or not re.search(r'project_id\s*=\s*braintrust_project\.', hcl) or not re.search(r'project_group_id\s*=\s*braintrust_project_group\.', hcl)):
                    failures.append(f"{name}: separate memberships or graph references missing")
                if name == "project_group_subset" and (SECOND_PROJECT not in hcl or "braintrust_project." not in hcl):
                    failures.append(f"{name}: omitted project should remain a raw ID while emitted project is linked")
                if name == "project_group_membership_subset" and (not re.search(r'project_id\s*=\s*"' + re.escape(SECOND_PROJECT) + r'"', hcl) or not re.search(r'project_id\s*=\s*braintrust_project\.', hcl) or "member_projects" in hcl):
                    failures.append(f"{name}: unselected project membership should retain a raw ID without double management")
                if name == "project_group_import" and ("braintrust_project_group" not in hcl or PROJECT_GROUP not in hcl):
                    failures.append(f"{name}: project group import block omitted")
                if name == "project_group_membership_import" and ("braintrust_project_group_membership" not in hcl or PROJECT_GROUP + "/" + PROJECT not in hcl or "member_projects" in hcl):
                    failures.append(f"{name}: membership adoption is incomplete or double-managed")
                if name == "export_automation_config" and (
                    "export_config" not in hcl
                    or "aws_iam_credentials" not in hcl
                    or not re.search(r'external_id\s*=\s*"fixture-external-id"', hcl)
                    or not re.search(r'btql_query\s*=\s*"SELECT \* FROM project_logs"', hcl)
                ):
                    failures.append(f"{name}: required S3 export fields omitted")
                if name == "function_generation" and (hcl.count('resource "braintrust_function"') != 3 or "function_data_json" not in hcl or "prompt_data_json" not in hcl or "braintrust_project." not in hcl or "tags" not in hcl or "preprocessor" not in hcl or "scorer" not in hcl):
                    failures.append(f"{name}: prompt, scorer, or preprocessor definition omitted")
                if name == "function_import" and (hcl.count('resource "braintrust_function"') != 3 or hcl.count('to = braintrust_function.') != 3):
                    failures.append(f"{name}: function adoption omitted resources or import blocks")
                if name == "dataset_definition" and ("metadata_json" not in hcl or "tags" not in hcl):
                    failures.append(f"{name}: dataset metadata or tags omitted")
                if name == "score_definition" and ("categories_json" not in hcl or "config_json" not in hcl):
                    failures.append(f"{name}: score definition fields omitted")
                if name == "view_definition" and ("custom_charts" not in hcl or "view_data_json" not in hcl or not re.search(r'object_id\s*=\s*braintrust_project\.', hcl) or not re.search(r'object_type\s*=\s*"org_project"', hcl) or not re.search(r'"projectId"\s*:\s*braintrust_project\.', hcl)):
                    failures.append(f"{name}: scoped dashboard or saved view omitted")
                if name == "view_import" and f"org_project/{ORG}/00000000-0000-4000-8000-000000000032" not in hcl:
                    failures.append(f"{name}: scope-aware dashboard import ID omitted")
                if name == "new_resources" and (
                    'resource "braintrust_org_automation"' not in hcl
                    or 'resource "braintrust_mcp_server"' not in hcl
                    or 'retention_days = 0' not in hcl
                    or 'project_id = braintrust_project.' not in hcl
                    or 'other-org-retention' in hcl or 'unselected-mcp' in hcl or 'deleted-mcp' in hcl
                    or hcl.count('import {') != 3
                ):
                    failures.append(f"{name}: new resources, scope filtering, or imports incorrect")
                if name == "multi_project_fetch":
                    for path in ("dataset", "function", "project_score", "project_tag", "project_automation"):
                        if sum(urlparse(request).path == "/v1/" + path for request in requests) != 1:
                            failures.append(f"{name}: {path} was listed more than once")
                    if hcl.count('resource "braintrust_dataset"') != 2 or "other-org-dataset" in hcl or hcl.count('resource "braintrust_project_automation"') != 1:
                        failures.append(f"{name}: selected project resources were duplicated or crossed organizations")
                if name == "builtin_role_reference" and not re.search(r'role_id\s*=\s*data\.braintrust_role\.', hcl):
                    failures.append(f"{name}: built-in role has no data source reference")
                if name == "builtin_role_reference" and not re.search(r'object_type\s*=\s*"prompt"', hcl):
                    failures.append(f"{name}: org-wide prompt ACL omitted")
                if name.startswith("builtin_group_"):
                    if re.search(r'resource "braintrust_group" "[^"]+"\s*\{[^}]*name\s*=\s*"(?:All AI Provider Access|Engineers)"', hcl):
                        failures.append(f"{name}: built-in group was declared as managed")
                    if hcl.count('resource "braintrust_group_membership"') != 2:
                        failures.append(f"{name}: human memberships in built-in groups were not captured")
                    if SERVICE_USER in hcl or "45000000-0000-4000-8000-000000000045" in hcl:
                        failures.append(f"{name}: service membership or default ACL became managed")
                    if name == "builtin_group_import" and hcl.count('import {') != 6:
                        failures.append(f"{name}: expected custom group, two human memberships, and three ACL imports")
                    if name == "builtin_group_membership" and (not re.search(r'group_id\s*=\s*data\.braintrust_group\.', hcl) or not re.search(r'user_id\s*=\s*data\.braintrust_user\.', hcl)):
                        failures.append(f"{name}: built-in membership lacks stable data source references")
                    if name == "builtin_group_no_links" and (re.search(r'group_id\s*=\s*data\.braintrust_group\.', hcl) or re.search(r'user_id\s*=\s*data\.braintrust_user\.', hcl)):
                        failures.append(f"{name}: raw-ID mode unexpectedly linked group membership")
                    if "skipped 1 distinct built-in group member" not in generated.stderr:
                        failures.append(f"{name}: skipped service membership was not reported")
                if name == "raw_acl_user" and not re.search(r'user_id\s*=\s*"' + re.escape(USER) + r'"', hcl):
                    failures.append(f"{name}: raw ACL user ID omitted")
                if name == "subset_project_reference" and "braintrust_project." in hcl:
                    failures.append(f"{name}: references a project absent from the selected resource types")
                if name == "service_token_import":
                    if 'resource "' in hcl or 'import {' in hcl or "omitted 1" not in generated.stderr:
                        failures.append(f"{name}: unsupported import was not explicitly omitted")
                if name == "secret_variable_syntax" and (not re.search(r'type\s*=\s*string', hcl) or not re.search(r'sensitive\s*=\s*true', hcl) or not re.search(r'secret\s*=\s*var\.braintrust_ai_secret_', hcl)):
                    failures.append(f"{name}: AI secret has no required sensitive variable reference")
                if name == "secret_variable_collision":
                    declarations = re.findall(r'variable "([^"]+)"', hcl)
                    if len(declarations) != 2 or len(set(declarations)) != 2:
                        failures.append(f"{name}: secret variable names collided")
                if name == "ordinary_env_var_value" and not re.search(r'value\s*=\s*var\.braintrust_env_var_', hcl):
                    failures.append(f"{name}: write-only ordinary env var value has no variable")
                if name == "ordinary_env_var_value" and not re.search(r'object_id\s*=\s*braintrust_project\.', hcl):
                    failures.append(f"{name}: project env var still points to a source-org project ID")
                if name in {"secret_variable_syntax", "ordinary_env_var_value"} and "required sensitive variable(s)" not in generated.stderr:
                    failures.append(f"{name}: required secret variables were not reported")
                if name == "ai_secret_adoption" and ('variable "' in hcl or re.search(r'secret\s*=\s*var\.', hcl)):
                    failures.append(f"{name}: import configuration would rotate an unknown write-only value")
                if name == "stale_acl_omission" and ('resource "braintrust_acl" "terraform_managed_resource_50000000"' in hcl or "skipped 1 ACL" not in generated.stderr):
                    failures.append(f"{name}: confirmed dead ACL was exported or omitted silently")
                if name == "stale_acl_unverified" and ('resource "braintrust_acl" "terraform_managed_resource_50000000"' not in hcl or "ACL target(s) were not verified" not in generated.stderr):
                    failures.append(f"{name}: unverified ACL was omitted or warning is missing")
                if name == "stale_acl_omission" and not acl_reads:
                    failures.append(f"{name}: ACL verification made no individual reads")
                if name == "multi_org_selected" and "other-org-" in hcl:
                    failures.append(f"{name}: emitted resource from another org")
                if name == "empty_org_env_var" and "ORG_VALUE" not in hcl:
                    failures.append(f"{name}: omitted org env var when org has no projects")
            if provider:
                state["fixtures"] = copy.deepcopy(FIXTURES)
                state["fixtures"]["/v1/group"] = [{
                    "id": BUILTIN_GROUP, "org_id": ORG, "name": "All AI Provider Access",
                    "member_users": [SERVICE_USER],
                }]
                workspace = root / "group_membership_lifecycle"
                workspace.mkdir()
                (workspace / "main.tf").write_text(
                    'terraform {\n  required_providers {\n    braintrust = { source = "braintrustdata/braintrust" }\n  }\n}\n'
                    'provider "braintrust" {}\n'
                    'resource "braintrust_group_membership" "human" {\n'
                    f'  group_id = "{BUILTIN_GROUP}"\n  user_id = "{USER}"\n'
                    '}\n'
                )
                applied = subprocess.run(
                    ["terraform", "apply", "-auto-approve", "-input=false", "-no-color"],
                    cwd=workspace, env=env, capture_output=True, text=True, timeout=60,
                )
                if applied.returncode != 0:
                    failures.append("group_membership_lifecycle: apply failed: " + applied.stderr[-500:])
                elif set(state["fixtures"]["/v1/group"][0]["member_users"]) != {USER, SERVICE_USER}:
                    failures.append("group_membership_lifecycle: adding the human changed the service account membership")
                destroyed = subprocess.run(
                    ["terraform", "destroy", "-auto-approve", "-input=false", "-no-color"],
                    cwd=workspace, env=env, capture_output=True, text=True, timeout=60,
                ) if applied.returncode == 0 else None
                if destroyed and destroyed.returncode != 0:
                    failures.append("group_membership_lifecycle: destroy failed: " + destroyed.stderr[-500:])
                elif destroyed and state["fixtures"]["/v1/group"][0]["member_users"] != [SERVICE_USER]:
                    failures.append("group_membership_lifecycle: destroy removed another member or left the human member")
                destination = args.output / "group_membership_lifecycle"
                destination.mkdir(exist_ok=True)
                (destination / "apply.stdout.txt").write_text(applied.stdout)
                (destination / "apply.stderr.txt").write_text(applied.stderr)
                if destroyed:
                    (destination / "destroy.stdout.txt").write_text(destroyed.stdout)
                    (destination / "destroy.stderr.txt").write_text(destroyed.stderr)
                print("group_membership_lifecycle apply", applied.returncode, "destroy", destroyed.returncode if destroyed else "skipped")
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
    (args.output / "results.json").write_text(json.dumps(results, indent=2) + "\n")
    print("Evidence:", args.output.resolve())
    if failures:
        raise SystemExit("Offline regression failures:\n" + "\n".join(failures))


if __name__ == "__main__":
    main()
