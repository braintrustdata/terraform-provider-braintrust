#!/usr/bin/env python3
"""Maintain one review issue for API changes detected by check_api_drift.py."""

import json
import os
from pathlib import Path
from urllib.request import Request, urlopen


MARKER = "<!-- braintrust-api-drift -->"
TITLE = "Review Braintrust API changes affecting the Terraform provider"


def api(method: str, path: str, data=None):
    body = json.dumps(data).encode() if data is not None else None
    req = Request(
        f"https://api.github.com/repos/{os.environ['GITHUB_REPOSITORY']}/{path}",
        data=body,
        headers={"Authorization": f"Bearer {os.environ['GITHUB_TOKEN']}", "Accept": "application/vnd.github+json", "Content-Type": "application/json", "X-GitHub-Api-Version": "2022-11-28"},
        method=method,
    )
    with urlopen(req, timeout=30) as response:
        return json.load(response)


def main():
    drift = json.loads(Path(os.environ["DRIFT_STATUS"]).read_text())["drift"]
    body = Path(os.environ["DRIFT_BODY"]).read_text()
    issues = []
    page = 1
    while True:
        batch = api("GET", f"issues?state=open&per_page=100&page={page}")
        issues.extend(issue for issue in batch if "pull_request" not in issue)
        if len(batch) < 100:
            break
        page += 1
    existing = next((issue for issue in issues if MARKER in (issue.get("body") or "")), None)
    if drift:
        if existing:
            if existing["body"] != body:
                api("PATCH", f"issues/{existing['number']}", {"body": body})
                print(f"Updated issue #{existing['number']}")
            else:
                print(f"Issue #{existing['number']} is current")
        else:
            created = api("POST", "issues", {"title": TITLE, "body": body})
            print(f"Created issue #{created['number']}")
    elif existing:
        api("PATCH", f"issues/{existing['number']}", {"state": "closed", "state_reason": "completed"})
        print(f"Closed resolved drift issue #{existing['number']}")
    else:
        print("No drift and no open drift issue")


if __name__ == "__main__":
    main()
