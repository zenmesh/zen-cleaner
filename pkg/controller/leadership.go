/*
Copyright 2026 Zen Mesh

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// SUPPORT2-003 §11-§12: leadership-aware health semantics.
//
// Process health (liveness/readiness) must reflect whether the pod can
// serve — as leader OR as healthy standby — not whether it currently owns
// the lease. Leader state is observable separately (/leaderz) and via the
// lease holder. This removes the SUPPORT2-002 P3 wart where standby pods
// reported NotReady solely because the health server started only while
// leading, which broke Deployment rollout/readiness tooling without adding
// any safety (client-go leader election already guarantees exactly-one
// reconciler).
package controller

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	sdklog "github.com/zenmesh/zen-cleaner/internal/logging"
)

// LeaderState tracks whether this replica currently leads, plus a startup
// grace window during which a not-yet-started controller is not penalized.
type LeaderState struct {
	leading      atomic.Bool
	startedAt    time.Time
	logger       *sdklog.Logger
	startupGrace time.Duration
}

// NewLeaderState creates the tracker with a startup grace period.
func NewLeaderState(startupGrace time.Duration, logger *sdklog.Logger) *LeaderState {
	return &LeaderState{
		startedAt:    time.Now(),
		logger:       logger,
		startupGrace: startupGrace,
	}
}

// SetLeading records a leadership transition.
func (l *LeaderState) SetLeading(v bool) { l.leading.Store(v) }

// IsLeading reports current leadership.
func (l *LeaderState) IsLeading() bool { return l.leading.Load() }

// InStartupGrace reports whether the process is still inside its startup
// grace window.
func (l *LeaderState) InStartupGrace() bool {
	return time.Since(l.startedAt) < l.startupGrace
}

// ReadinessCheck: leadership-aware readiness (SUPPORT2-003 §11-§12).
//   - leader: existing readiness law (informers synced etc.).
//   - standby: process-healthy readiness — a standby that can reach the API
//     and serve /readyz is a healthy standby; it does not reconcile because
//     client-go leader election prevents it, which is the actual safety law.
//   - neither leading nor in startup grace with no election result yet:
//     still Ready (it is serving as a warm standby) — readiness never claims
//     reconciliation.
func (h *HealthChecker) LeadershipAwareReadinessCheck(leader *LeaderState) func(req *http.Request) error {
	return func(req *http.Request) error {
		if leader != nil && !leader.IsLeading() {
			// Standby: process can serve; reconciliation is lease-gated.
			return nil
		}
		// Leader (or election disabled): full readiness law.
		return h.ReadinessCheck(req)
	}
}

// LeadershipAwareLivenessCheck: liveness must never fail a healthy standby.
// On standby there is no evaluation activity by design, so the
// evaluation-recency liveness law only applies while leading.
func (h *HealthChecker) LeadershipAwareLivenessCheck(leader *LeaderState) func(req *http.Request) error {
	return func(req *http.Request) error {
		if leader != nil && !leader.IsLeading() {
			return nil
		}
		return h.LivenessCheck(req)
	}
}

// LeaderzCheck reports the current leadership state as an HTTP check body.
func LeaderzCheck(leader *LeaderState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if leader == nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("unknown"))
			return
		}
		w.WriteHeader(http.StatusOK)
		state := "standby"
		if leader.IsLeading() {
			state = "leader"
		}
		_, _ = fmt.Fprint(w, state)
	}
}

// ServeStandalone starts the always-on health server for ALL replicas
// (leader and standby) BEFORE leader election. This is the fix for the
// SUPPORT2-002 P3: Deployment readiness converges without claiming that a
// standby is reconciling. Returns a stop function.
func ServeStandalone(ctx context.Context, addr string, mux *http.ServeMux) (stop func(), err error) {
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		case err := <-errCh:
			if err != nil {
				sdklog.NewLogger("zen-cleaner").Error(err, "standalone health server failed")
			}
		}
	}()
	return func() { _ = srv.Shutdown(context.Background()) }, nil
}

var _ = context.Background
