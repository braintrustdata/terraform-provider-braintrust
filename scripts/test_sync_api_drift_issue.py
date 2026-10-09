import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import sync_api_drift_issue


class IssueSyncTest(unittest.TestCase):
    def run_sync(self, drift, issues):
        with tempfile.TemporaryDirectory() as directory:
            status = Path(directory) / "status.json"
            body = Path(directory) / "body.md"
            status.write_text(json.dumps({"drift": drift}))
            body.write_text("<!-- braintrust-api-drift -->\nNew contract\n" if drift else "")
            calls = []

            def fake_api(method, path, data=None):
                calls.append((method, path, data))
                if method == "GET":
                    return issues
                return {"number": 42}

            with patch.dict(os.environ, {"DRIFT_STATUS": str(status), "DRIFT_BODY": str(body)}), patch.object(sync_api_drift_issue, "api", fake_api):
                sync_api_drift_issue.main()
            return calls

    def test_creates_one_issue_for_new_drift(self):
        calls = self.run_sync(True, [])
        self.assertEqual(calls[-1][0:2], ("POST", "issues"))

    def test_updates_existing_issue_in_place(self):
        calls = self.run_sync(True, [{"number": 7, "body": "<!-- braintrust-api-drift -->\nOld contract\n"}])
        self.assertEqual(calls[-1][0:2], ("PATCH", "issues/7"))

    def test_closes_resolved_issue(self):
        calls = self.run_sync(False, [{"number": 7, "body": "<!-- braintrust-api-drift -->\nOld contract\n"}])
        self.assertEqual(calls[-1], ("PATCH", "issues/7", {"state": "closed", "state_reason": "completed"}))


if __name__ == "__main__":
    unittest.main()
