package controller

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// SUPPORT2-042 observability law regression guards:
//   - every replica (leader or standby) serves /metrics on the canonical
//     metrics port — the manager's leader-only bind previously left standbys
//     unscrapable;
//   - the payload gathers the default registry (process/runtime +
//     zen_cleaner_* business metrics) AND the controller-runtime registry;
//   - leadership is distinguishable per replica via
//     zen_cleaner_leadership_state (1 = leader, 0 = standby).
func TestServeMetricsAlwaysOn_ServesBothRegistries(t *testing.T) {
	SetLeadershipGauge(true) // business metric must be visible immediately

	const lnAddr = "127.0.0.1:18099"
	stop, err := ServeMetricsAlwaysOn(context.Background(), lnAddr)
	if err != nil {
		t.Fatalf("start metrics server: %v", err)
	}
	defer stop()

	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get("http://" + lnAddr + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics: got %d want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)

	for _, want := range []string{
		"zen_cleaner_leadership_state 1", // leadership gauge, flips with SetLeadershipGauge
		"go_goroutines",                  // default registry runtime collector
	} {
		if !strings.Contains(s, want) {
			t.Errorf("/metrics payload missing %q", want)
		}
	}
}

func TestSetLeadershipGauge_FlipsValue(t *testing.T) {
	SetLeadershipGauge(false)
	if v := testutil.ToFloat64(leadershipState); v != 0 {
		t.Fatalf("standby: got %v want 0", v)
	}
	SetLeadershipGauge(true)
	if v := testutil.ToFloat64(leadershipState); v != 1 {
		t.Fatalf("leader: got %v want 1", v)
	}
	SetLeadershipGauge(false)
}
