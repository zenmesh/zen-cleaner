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

package webhook

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// The webhook metrics are registered on controller-runtime's metrics
// Registry (the same surface pkg/controller/metrics.go uses via
// promauto.With) so they are exported by the existing always-on
// /metrics server (ServeMetricsAlwaysOn) without a second port or a
// second registry — one gather, no duplicate go_*/process_* collectors.
var (
	webhookRequestsTotal = promauto.With(ctrlmetrics.Registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "zen_cleaner_webhook_requests_total",
			Help: "Total admission webhook requests handled, by endpoint and HTTP response code.",
		},
		[]string{"endpoint", "code"},
	)

	webhookRequestDuration = promauto.With(ctrlmetrics.Registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "zen_cleaner_webhook_request_duration_seconds",
			Help:    "Admission webhook request handling duration in seconds, by endpoint.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
		},
		[]string{"endpoint"},
	)
)

// instrument wraps a webhook handler with request count and duration
// accounting for the given logical endpoint name.
func instrument(endpoint string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		webhookRequestsTotal.WithLabelValues(endpoint, strconv.Itoa(sw.code)).Inc()
		webhookRequestDuration.WithLabelValues(endpoint).Observe(time.Since(start).Seconds())
	})
}

// statusWriter captures the response status code for the metrics while
// passing everything else through untouched.
type statusWriter struct {
	http.ResponseWriter
	code    int
	wroteCH bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wroteCH {
		sw.code = code
		sw.wroteCH = true
	}
	sw.ResponseWriter.WriteHeader(code)
}
