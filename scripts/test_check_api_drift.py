import unittest

from check_api_drift import contract, differences, issue_body


class ContractTest(unittest.TestCase):
    def spec(self, property_type="string"):
        return {
            "paths": {
                "/v1/view": {
                    "post": {
                        "requestBody": {
                            "content": {
                                "application/json": {
                                    "schema": {"$ref": "#/components/schemas/CreateView"}
                                }
                            }
                        }
                    }
                }
            },
            "components": {
                "schemas": {
                    "CreateView": {
                        "type": "object", "properties": {"name": {"type": property_type}}
                    }
                }
            },
        }

    def test_schema_change_becomes_actionable_issue(self):
        before = contract(self.spec())
        after = contract(self.spec("integer"))
        changes = differences(before, after)
        self.assertEqual([(section, name) for section, name, _, _ in changes], [("schemas", "CreateView")])
        body = issue_body(changes, "abc123")
        self.assertIn("CreateView", body)
        self.assertIn("+      \"type\": \"integer\"", body)
        self.assertIn("braintrust-api-drift", body)

    def test_descriptions_do_not_create_noise(self):
        spec = self.spec()
        before = contract(spec)
        spec["components"]["schemas"]["CreateView"]["description"] = "new docs"
        self.assertEqual(differences(before, contract(spec)), [])

    def test_function_endpoint_is_tracked(self):
        spec = self.spec()
        spec["paths"]["/v1/function"] = {
            "post": {"requestBody": {"content": {"application/json": {
                "schema": {"$ref": "#/components/schemas/CreateFunction"}
            }}}}
        }
        spec["components"]["schemas"]["CreateFunction"] = {
            "type": "object", "properties": {"function_data": {"type": "object"}}
        }
        tracked = contract(spec)
        self.assertIn("/v1/function", tracked["paths"])
        self.assertIn("CreateFunction", tracked["schemas"])


if __name__ == "__main__":
    unittest.main()
