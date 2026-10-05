// Copyright 2026 Zen Mesh. All rights reserved.

package clireadops

import (
	"strings"
	"testing"
)

func TestEntitlementStatusRender(t *testing.T) {
	s := EntitlementStatus{EntitlementKey: "cleaner.executions", MeteredUsage: 7}
	out := s.Render()
	if !strings.Contains(out, "cleaner.executions") {
		t.Fatalf("the entitlement key missing: %q", out)
	}
	if !strings.Contains(out, "7") {
		t.Fatalf("the metered usage missing: %q", out)
	}
}

func TestHealthSummaryRender(t *testing.T) {
	s := HealthSummary{Version: "v1.2.3", Commit: "abc1234", ProcessUptime: "4m"}
	out := s.Render()
	for _, want := range []string{"v1.2.3", "abc1234", "uptime"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the summary missing %q: %q", want, out)
		}
	}
}

func TestNoClusterDataInSummaries(t *testing.T) {
	// THE LAW: the summaries read ONLY the process-local facts — never
	// the cluster data (the cluster data is the API server's to serve).
	s := HealthSummary{Version: "v", Commit: "c", ProcessUptime: "1m"}.Render()
	for _, banned := range []string{"namespace", "pod", "node", "cluster"} {
		if strings.Contains(strings.ToLower(s), banned) {
			t.Fatalf("the health summary must not carry cluster data: %q in %q", banned, s)
		}
	}
}
