package main

import (
	"os"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/cmd"
)

// version is set at build time via ldflags.
var version = "dev"

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
