package main

import (
	"flag"
	"fmt"
	"github.com/zenmesh/zen-cleaner/internal/clireadops"
)

var healthSummaryVerb = flag.Bool("health-summary", false, "Print process-local version, commit and uptime")

func runStatusVerbs() bool {
	if !*healthSummaryVerb {
		return false
	}
	fmt.Print(clireadops.ReadHealthSummary(version, commit).Render())
	return true
}
