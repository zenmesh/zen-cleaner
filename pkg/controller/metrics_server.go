package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// SUPPORT2-042 observability law: EVERY replica (leader and standby) exposes
// /metrics on the canonical metrics port, so Prometheus scrapes succeed for
// all pods of the deployment. The controller-runtime manager's own metrics
// server is disabled (BindAddress "0") because it only starts on the leader,
// which previously left standbys unscrapable. Exactly one server owns the
// port per replica — no double-bind.
//
// The served payload is controller-runtime's metrics.Registry, which already
// carries the Go runtime/process collectors, controller/workqueue metrics,
// and all zen_cleaner_* business metrics (metrics.go registers via
// promauto.With(metrics.Registry)). The default prometheus registry is
// intentionally NOT gathered alongside it: both carry go_*/process_*
// collectors and a combined gather fails with duplicate-metric errors.
//
// Leadership remains distinguishable per replica via
// zen_cleaner_leadership_state (1 = leader, 0 = standby); singleton
// reconciliation work is still leader-only (client-go lease), unchanged.

var leadershipState = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "zen_cleaner_leadership_state",
	Help: "1 when this replica holds the leader lease, 0 when it is a standby.",
})

func init() {
	ctrlmetrics.Registry.MustRegister(leadershipState)
	leadershipState.Set(0)
}

// SetLeadershipGauge records a leadership transition in the metrics surface.
// Wired alongside LeaderState.SetLeading via OnLeadingChange.
func SetLeadershipGauge(leading bool) {
	if leading {
		leadershipState.Set(1)
		return
	}
	leadershipState.Set(0)
}

// ServeMetricsAlwaysOn starts the all-replica /metrics server. Returns a stop
// function. ReadTimeout/ReadHeaderTimeout bound slow-client exposure.
func ServeMetricsAlwaysOn(ctx context.Context, addr string) (stop func(), err error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(ctrlmetrics.Registry, promhttp.HandlerOpts{}))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if serveErr := srv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
			return
		}
		close(errCh)
	}()

	select {
	case serveErr := <-errCh:
		if serveErr != nil {
			return nil, serveErr
		}
	case <-time.After(50 * time.Millisecond):
		// listen succeeded (no immediate error surfaced)
	}

	stopFn := func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}
	return stopFn, nil
}
