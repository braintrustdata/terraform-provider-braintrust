// Package openapispec embeds the Braintrust OpenAPI spec that the provider is
// generated from, so other tools in this repo can read the same copy.
package openapispec

import _ "embed"

//go:embed openapi_spec.json
var JSON []byte
