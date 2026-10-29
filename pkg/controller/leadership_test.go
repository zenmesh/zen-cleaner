package controller

// 2026-10-29 launch validation: the enforcement-failure mode (NetworkPolicy
// rejecting the API dial) left a pod reporting Ready with only client-go
// klog evidence. The readiness law must treat API unreachability as its own
// failure class while never failing a reachable standby for not leading.
import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadinessUnreachableStandbyIsAFailure(t *testing.T) {
	ls := NewLeaderState(0, nil)
	ls.SetLeading(false)
	ls.SetAPIReachable(false, errors.New("dial tcp 10.43.0.1:443: connect: connection refused"))
	h := &HealthChecker{}
	err := h.LeadershipAwareReadinessCheck(ls)(nil)
	if err == nil {
		t.Fatal("unreachable standby must NOT report Ready")
	}
	if !strings.Contains(err.Error(), "API unreachable") {
		t.Fatalf("readiness failure must name API unreachability, got: %v", err)
	}
}

func TestReadinessReachableStandbyStaysReady(t *testing.T) {
	ls := NewLeaderState(0, nil)
	ls.SetLeading(false)
	ls.SetAPIReachable(true, nil)
	h := &HealthChecker{}
	if err := h.LeadershipAwareReadinessCheck(ls)(nil); err != nil {
		t.Fatalf("reachable standby is a healthy standby (§11-§12): %v", err)
	}
}

func TestLeaderzReportsUnreachable(t *testing.T) {
	ls := NewLeaderState(0, nil)
	ls.SetLeading(true)
	ls.SetAPIReachable(false, errors.New("refused"))
	w := &stringWriter{header: http.Header{}}
	r := httptest.NewRequest("GET", "/leaderz", nil)
	LeaderzCheck(ls).ServeHTTP(w, r)
	if !strings.Contains(w.body, "API unreachable") {
		t.Fatalf("/leaderz must surface API unreachability, got: %q", w.body)
	}
}

func TestSetAPIReachableFlipsState(t *testing.T) {
	ls := NewLeaderState(0, nil)
	if !ls.APIReachable() {
		t.Fatal("optimistic default must be reachable")
	}
	ls.SetAPIReachable(false, errors.New("x"))
	if ls.APIReachable() {
		t.Fatal("state must flip to unreachable")
	}
	if ls.InStartupGrace() && time.Since(ls.startedAt) > time.Minute {
		t.Fatal("grace window inconsistent")
	}
}

type stringWriter struct {
	body   string
	header http.Header
}

func (w *stringWriter) Header() http.Header         { return w.header }
func (w *stringWriter) Write(p []byte) (int, error) { w.body += string(p); return len(p), nil }
func (w *stringWriter) WriteHeader(int)             {}
