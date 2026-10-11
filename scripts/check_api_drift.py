#!/usr/bin/env python3
"""Compare the public Braintrust OpenAPI contract with the provider's baseline.

The upstream spec is synced from Braintrust's internal repository. This check
only reports changes; provider code and baselines still need human review.
"""

from __future__ import annotations

import argparse
import difflib
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
BASELINE = ROOT / "api-contract-baseline.json"
TRACKED = (
    "project", "dataset", "prompt", "function", "group", "project_group", "role",
    "acl", "env_var", "project_tag", "project_score", "service_account",
    "service_token", "ai_secret", "project_automation", "view", "organization",
)
DOCUMENTATION_KEYS = {"description", "summary", "externalDocs", "examples", "example", "tags", "operationId"}


def contract(spec: dict) -> dict:
    paths = spec["paths"]
    schemas = spec["components"]["schemas"]
    parameters = spec["components"].get("parameters", {})
    selected_paths = {}
    refs = set()

    def normalize(value):
        if isinstance(value, dict):
            if "$ref" in value:
                ref = value["$ref"]
                if ref.startswith("#/components/schemas/"):
                    refs.add(ref.rsplit("/", 1)[1])
                    return {"$ref": ref}
                if ref.startswith("#/components/parameters/"):
                    return normalize(parameters[ref.rsplit("/", 1)[1]])
            return {key: normalize(child) for key, child in sorted(value.items()) if key not in DOCUMENTATION_KEYS}
        if isinstance(value, list):
            return [normalize(child) for child in value]
        return value

    for path, methods in sorted(paths.items()):
        if not any(path == f"/v1/{name}" or path.startswith(f"/v1/{name}/") for name in TRACKED):
            continue
        selected_paths[path] = {method: normalize(operation) for method, operation in sorted(methods.items()) if method.lower() not in {"options", "parameters"}}

    selected_schemas = {}
    while refs:
        name = refs.pop()
        if name not in selected_schemas:
            selected_schemas[name] = normalize(schemas[name])

    return {
        "paths": selected_paths,
        "schemas": dict(sorted(selected_schemas.items())),
        "all_api_paths": sorted(p for p in paths if p.startswith("/v1/") and not p.endswith("/options")),
    }


def differences(old: dict, new: dict) -> list[tuple[str, str, object, object]]:
    result = []
    for section in ("paths", "schemas"):
        before, after = old[section], new[section]
        for key in sorted(before.keys() | after.keys()):
            if before.get(key) != after.get(key):
                result.append((section, key, before.get(key), after.get(key)))
    old_paths, new_paths = set(old["all_api_paths"]), set(new["all_api_paths"])
    for path in sorted(old_paths ^ new_paths):
        result.append(("api_paths", path, path if path in old_paths else None, path if path in new_paths else None))
    return result


def issue_body(changes: list[tuple[str, str, object, object]], upstream_sha: str) -> str:
    link = f"https://github.com/braintrustdata/braintrust-openapi/tree/{upstream_sha or 'main'}/openapi"
    lines = ["<!-- braintrust-api-drift -->", "# Braintrust API contract changed", "",
             f"Source: [braintrust-openapi]({link}) (`{upstream_sha or 'main'}`).", "",
             "Review each change against the provider client, resource schemas, docs, terraforming exporter, and acceptance tests. Update the baseline only after handling or explicitly deferring the change.", ""]
    for section, key, before, after in changes:
        status = "added" if before is None else "removed" if after is None else "changed"
        lines.append(f"- **{section}: `{key}`** — {status}")
    lines.append("")
    lines.append("<details><summary>Contract diff</summary>")
    lines.append("")
    lines.append("```diff")
    for section, key, before, after in changes:
        if section == "api_paths":
            continue
        before_lines = json.dumps(before, indent=2, sort_keys=True).splitlines()
        after_lines = json.dumps(after, indent=2, sort_keys=True).splitlines()
        delta = list(difflib.unified_diff(before_lines, after_lines, fromfile=f"before/{section}/{key}", tofile=f"after/{section}/{key}", lineterm=""))
        lines.extend(delta[:100])
        if len(delta) > 100:
            lines.append(f"... {len(delta) - 100} more diff lines for {key}")
    footer = "\n```\n</details>\n\nThis issue is updated in place by `.github/workflows/api-drift.yml`.\n"
    body = "\n".join(lines)
    # GitHub issue bodies have a size limit; keep the action summary complete
    # and trim only the expanded diff when a large spec sync lands.
    if len(body.encode()) > 60000:
        body = body[:55000] + "\n... diff truncated; inspect the linked upstream spec."
    return body + footer


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--spec", type=Path, required=True)
    parser.add_argument("--baseline", type=Path, default=BASELINE)
    parser.add_argument("--refresh-baseline", action="store_true")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--status", type=Path)
    parser.add_argument("--upstream-sha", default="")
    args = parser.parse_args()
    current = contract(json.loads(args.spec.read_text()))
    if args.refresh_baseline:
        args.baseline.write_text(json.dumps(current, indent=2, sort_keys=True) + "\n")
        print(f"Updated {args.baseline}")
        return
    baseline = json.loads(args.baseline.read_text())
    changes = differences(baseline, current)
    if args.output:
        args.output.write_text(issue_body(changes, args.upstream_sha) if changes else "")
    if args.status:
        args.status.write_text(json.dumps({"drift": bool(changes), "count": len(changes)}) + "\n")
    print(f"{len(changes)} API contract change(s)")


if __name__ == "__main__":
    main()
