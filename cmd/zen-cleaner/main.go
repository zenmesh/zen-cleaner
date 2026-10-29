/*
Copyright 2025 Zen Mesh

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

// Package main implements the Zen Cleaner controller command-line application.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/zenmesh/zen-cleaner/internal/election"
	cleanerlog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/internal/mcpserver"
	"github.com/zenmesh/zen-cleaner/observability"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
	"github.com/zenmesh/zen-cleaner/pkg/controller"
	gcwebhook "github.com/zenmesh/zen-cleaner/pkg/webhook"
)

// ErrWebhookTLSCertificatesMissing indicates that webhook TLS certificates are missing.
var ErrWebhookTLSCertificatesMissing = errors.New("webhook TLS certificates not found")

const (
	// DefaultShutdownTimeout is the default timeout for graceful shutdown.
	DefaultShutdownTimeout = 30 * time.Second

	// DefaultBatchSize is the default batch size for deletions.
	DefaultBatchSize = 50

	// DefaultMaxConcurrentEvaluations is the default maximum number of concurrent policy evaluations.
	DefaultMaxConcurrentEvaluations = 5
)

var (
	// Version information (set via build flags).
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
	logger    *cleanerlog.Logger
	setupLog  *cleanerlog.Logger
)

var (
	metricsAddr              = flag.String("metrics-addr", ":8080", "The address the metric endpoint binds to")
	mcpStdio                 = flag.Bool("mcp-stdio", false, "Serve the MCP adapter over stdio (DEFAULT-DENY: requires ZEN_CLEANER_MCP_MODE=stdio + ZEN_CLEANER_MCP_ACTOR); exits after the stdio session closes")
	healthProbeAddr          = flag.String("health-probe-addr", ":8081", "The address the standalone health server binds to (serves /healthz, /readyz, /startup, /leaderz on leader AND standby)")
	webhookAddr              = flag.String("webhook-addr", ":9443", "The address the webhook endpoint binds to")
	webhookCertFile          = flag.String("webhook-cert-file", "/etc/webhook/certs/tls.crt", "Path to TLS certificate file")
	webhookKeyFile           = flag.String("webhook-key-file", "/etc/webhook/certs/tls.key", "Path to TLS private key file")
	leaderElection           = flag.Bool("leader-election", true, "Enable leader election (recommended for multi-replica)")
	leaderElectionID         = flag.String("leader-election-id", "zen-cleaner-leader-election", "The ID for leader election")
	leaderElectionNamespace  = flag.String("leader-election-namespace", "default", "The namespace for leader election lock")
	enableWebhook            = flag.Bool("enable-webhook", true, "Enable validating webhook server")
	insecureWebhook          = flag.Bool("insecure-webhook", false, "Allow webhook to start without TLS (testing only)")
	cleanupInterval          = flag.Duration("cleanup-interval", 1*time.Minute, "Interval between cleanup evaluation runs")
	maxDeletionsPerSecond    = flag.Int("max-deletions-per-second", 10, "Default maximum deletions per second")
	batchSize                = flag.Int("batch-size", DefaultBatchSize, "Default batch size for deletions")
	maxConcurrentEvaluations = flag.Int("max-concurrent-evaluations", DefaultMaxConcurrentEvaluations, "Maximum number of policies to evaluate concurrently")
)

func main() {
	flag.Parse()
	os.Exit(runMain())
}

// runMCPStdio serves the MCP adapter's stdio transport and returns the
// process exit code (the session IS the process: the stdio closes, the
// process exits — the controller never starts).
func runMCPStdio() int {
	adapter := mcpserver.New(nil, nil)
	if !adapter.Enabled() {
		fmt.Fprintln(os.Stderr, "zen-cleaner: the MCP adapter is DEFAULT-DENIED (set ZEN_CLEANER_MCP_MODE=stdio and ZEN_CLEANER_MCP_ACTOR=<actor> to opt in)")
		return 1
	}
	if err := adapter.RunStdio(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "zen-cleaner: the MCP stdio session failed:", err)
		return 1
	}
	return 0
}

func runMain() int {
	// The MCP stdio mode (R051 N3 continuation): the adapter's own
	// transport, before ANY k8s client construction — the stdio session
	// is the process (DEFAULT-DENY: the adapter types the refusal).
	if *mcpStdio {
		return runMCPStdio()
	}

	// The read-only status verbs (the R051 parity build-out): print and
	// exit — the controller's run loop never starts for them.
	if runStatusVerbs() {
		return 0
	}

	// Initialize the zen-cleaner logger (configures the controller-runtime logger automatically)
	logger = cleanerlog.NewLogger("zen-cleaner")
	setupLog = logger.WithComponent("setup")
	setupLog.Debug("Zen Cleaner controller starting", cleanerlog.String("version", version), cleanerlog.String("commit", commit), cleanerlog.String("buildDate", buildDate))

	// OpenTelemetry tracing initialization can be added here when a tracing backend is configured
	// For now, continue without tracing

	// Get config using controller-runtime (handles kubeconfig flag automatically)
	restCfg := ctrl.GetConfigOrDie()

	dynamicClient, kubeClient, err := buildKubernetesClients(restCfg)
	if err != nil {
		return 1
	}

	// controller-runtime client for the reconciler (manager client is bound
	// per-leader inside runController; this one serves construction-time use).
	// SUPPORT2-003 §11: the reconciler is created for ALL replicas now.
	scheme, crClient, err := buildSchemeAndClient(restCfg)
	if err != nil {
		return 1
	}

	// Load controller configuration
	controllerConfig := config.NewControllerConfig()
	if err := controllerConfig.LoadFromEnv(); err != nil {
		setupLog.Error(err, "Error loading configuration from environment", cleanerlog.ErrorCode("CONFIG_LOAD_ERROR"))
		return 1
	}
	// Precedence: env (ZEN_CLEANER_*) overrides defaults; explicit flags
	// override env. Flags not explicitly set must not clobber env-provided
	// values, otherwise the documented ZEN_CLEANER_* contract is inert
	// whenever a deployment passes the flag defaults verbatim (SUPPORT2-002).
	applyFlagOverrides(controllerConfig)

	setupLog.Info("Controller configuration",
		cleanerlog.String("cleanupInterval", controllerConfig.CleanupInterval.String()),
		cleanerlog.Int("maxDeletionsPerSecond", controllerConfig.MaxDeletionsPerSecond),
		cleanerlog.Int("batchSize", controllerConfig.BatchSize),
		cleanerlog.Int("maxConcurrentEvaluations", controllerConfig.MaxConcurrentEvaluations))

	// Create status updater with configuration
	statusUpdater := controller.NewStatusUpdaterWithConfig(dynamicClient, controllerConfig)

	// Create event recorder
	eventRecorder := controller.NewEventRecorder(kubeClient)

	// Setup controller-runtime manager
	mgrOpts := newManagerOptions(scheme)

	// Set up graceful shutdown context
	ctx, cancel := election.ShutdownContext(context.Background(), "zen-cleaner")
	// SUPPORT2-049: OTel tracing. Safe default: no-op tracer unless
	// OTEL_EXPORTER_OTLP_ENDPOINT is set. Non-fatal on init failure.
	otel.SetTracerProvider(noop.NewTracerProvider())
	defer cancel()

	// Run with leader election using client-go
	leConfig := &election.Config{
		ElectionID: *leaderElectionID,
		Namespace:  *leaderElectionNamespace,
		Enable:     *leaderElection,
	}

	logLeaderElectionMode()

	// SUPPORT2-003 §11-§12: the reconciler and health checker are created
	// for ALL replicas (leader + standby), and a standalone always-on health
	// server serves /healthz, /readyz, /startup and /leaderz for every
	// replica. Readiness reflects "healthy leader OR healthy standby" —
	// never lease ownership — while exactly-one reconciliation remains
	// enforced by client-go leader election.
	leaderState := controller.NewLeaderState(15*time.Second, logger)

	// Discovery + RESTMapper for reliable GVR resolution and bounded
	// target-capability validation (SUPPORT2-003 §4).
	disc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		setupLog.Error(err, "Error building discovery client", cleanerlog.ErrorCode("CLIENT_ERROR"))
		return 1
	}
	restMapper, err := apiutil.NewDynamicRESTMapper(restCfg, http.DefaultClient)
	if err != nil {
		setupLog.Error(err, "Error building REST mapper", cleanerlog.ErrorCode("CLIENT_ERROR"))
		return 1
	}
	reconciler := controller.NewPolicyReconcilerWithRESTMapper(
		crClient,
		scheme,
		dynamicClient,
		restMapper,
		statusUpdater,
		eventRecorder,
		controllerConfig,
	)
	reconciler.SetDiscoveryClient(disc)

	healthChecker := controller.NewHealthChecker(reconciler)
	healthChecker.SetLeaderState(leaderState)

	// Standalone always-on health server (leader + standby).
	stopHealth, err := controller.ServeStandalone(ctx, *healthProbeAddr, newHealthMux(healthChecker, leaderState))
	if err != nil {
		setupLog.Error(err, "Error starting standalone health server", cleanerlog.ErrorCode("HEALTH_SERVER_ERROR"))
		return 1
	}
	defer stopHealth()

	// SUPPORT2-042 observability law: every replica serves /metrics on the
	// canonical metrics port (the manager's own bind is disabled above).
	// SUPPORT2-050: OTel tracing init (safe no-op default).
	if stopTrace := initTracing(ctx); stopTrace != nil {
		defer stopTrace()
	}

	stopMetrics, err := controller.ServeMetricsAlwaysOn(ctx, *metricsAddr)
	if err != nil {
		setupLog.Error(err, "Error starting always-on metrics server", cleanerlog.ErrorCode("METRICS_SERVER_ERROR"))
		return 1
	}
	defer stopMetrics()

	// Leadership transitions feed the health semantics (leader readiness is
	// the full law; standby readiness is process-level, per §11-§12) and the
	// per-replica leadership gauge exposed on /metrics.
	leConfig.OnLeadingChange = func(leading bool) {
		leaderState.SetLeading(leading)
		controller.SetLeadershipGauge(leading)
	}

	// API-reachability observation (2026-10-29 launch validation): every
	// leader-election lock operation reports whether the API was reached.
	// Unreachable is a distinct failure class (NetworkPolicy-enforced egress
	// rejection, RBAC denial, endpoint loss) surfaced as pod Events, a
	// metric, /leaderz detail and NOT-Ready /readyz — without conflating it
	// with follower status, which is never a failure.
	apiWasReachable := true
	leConfig.OnAPIObservation = func(reachable bool, err error) {
		leaderState.SetAPIReachable(reachable, err)
		controller.SetAPIReachableGauge(reachable)
		if reachable == apiWasReachable {
			return
		}
		apiWasReachable = reachable
		if podName := os.Getenv("POD_NAME"); podName != "" {
			ref := &corev1.ObjectReference{Kind: "Pod", Name: podName,
				Namespace: os.Getenv("POD_NAMESPACE")}
			if reachable {
				eventRecorder.Eventf(ref, corev1.EventTypeNormal, "APIReachable",
					"leader-election lock reachable again; API observation recovered")
			} else {
				detail := ""
				if err != nil {
					detail = err.Error()
				}
				eventRecorder.Eventf(ref, corev1.EventTypeWarning, "APIUnreachable",
					"leader-election lock unreachable; controller cannot reach the API: %s", detail)
			}
		}
	}

	// SUPPORT2-032R: admission is stateless and the webhook Service routes
	// to every ready replica, so every replica serves the webhook — not
	// only the leader (a leader-only webhook 502s whenever the apiserver's
	// dial lands on the standby under failurePolicy=Fail).
	startWebhookServer(ctx, gcwebhook.NewPolicyTargetGate(controllerConfig))

	err = election.RunWithLeaderElection(ctx, leConfig, kubeClient, func(runCtx context.Context) {
		runController(runCtx, restCfg, &mgrOpts, reconciler, healthChecker, scheme, dynamicClient, statusUpdater, eventRecorder, controllerConfig)
	})
	if err != nil {
		setupLog.Error(err, "Leader election failed", cleanerlog.ErrorCode("LEADER_ELECTION_ERROR"))
		return 1
	}
	return 0
}

// buildKubernetesClients creates the dynamic client (resource informers) and
// the Kubernetes client (events + leader election).
func buildKubernetesClients(restCfg *rest.Config) (*dynamic.DynamicClient, *kubernetes.Clientset, error) {
	// Create dynamic client (still needed for resource informers)
	dynamicClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		setupLog.Error(err, "Error building dynamic client", cleanerlog.ErrorCode("CLIENT_ERROR"))
		return nil, nil, err
	}

	// Create Kubernetes client for events + leader election
	kubeClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		setupLog.Error(err, "Error building Kubernetes client", cleanerlog.ErrorCode("CLIENT_ERROR"))
		return nil, nil, err
	}
	return dynamicClient, kubeClient, nil
}

// buildSchemeAndClient creates the scheme (with ZenCleanerPolicy types added)
// and the construction-time controller-runtime client.
func buildSchemeAndClient(restCfg *rest.Config) (*runtime.Scheme, client.Client, error) {
	// Create scheme and add ZenCleanerPolicy types
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "Error adding scheme", cleanerlog.ErrorCode("SCHEME_ERROR"))
		return nil, nil, err
	}

	crClient, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "Error building controller client", cleanerlog.ErrorCode("CLIENT_ERROR"))
		return nil, nil, err
	}
	return scheme, crClient, nil
}

// applyFlagOverrides applies explicitly-set flags over the env-loaded
// controller configuration.
func applyFlagOverrides(controllerConfig *config.ControllerConfig) {
	setFlags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })
	if setFlags["cleanup-interval"] {
		controllerConfig.WithCleanupInterval(*cleanupInterval)
	}
	if setFlags["max-deletions-per-second"] {
		controllerConfig.WithMaxDeletionsPerSecond(*maxDeletionsPerSecond)
	}
	if setFlags["batch-size"] {
		controllerConfig.WithBatchSize(*batchSize)
	}
	if setFlags["max-concurrent-evaluations"] {
		controllerConfig.WithMaxConcurrentEvaluations(*maxConcurrentEvaluations)
	}
}

// newManagerOptions builds the controller-runtime manager options (no leader
// election — we use client-go leader election).
func newManagerOptions(scheme *runtime.Scheme) ctrl.Options {
	return ctrl.Options{
		Scheme: scheme,
		// SUPPORT2-042 observability law: the manager's metrics server is
		// disabled because it starts only on the leader, which left standby
		// replicas unscrapable. The always-on standalone server binds
		// *metricsAddr on EVERY replica instead, gathering from the default
		// and controller-runtime registries (see ServeMetricsAlwaysOn).
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    9443,
			CertDir: "", // We'll handle webhook separately for now
		}),
		// SUPPORT2-003 §11: health endpoints are served by a standalone
		// always-on server for ALL replicas (leader + standby). The manager
		// does not bind its own probe port (avoids double-bind).
		HealthProbeBindAddress: "0",
	}
}

// logLeaderElectionMode reports whether leader election is enabled.
func logLeaderElectionMode() {
	if *leaderElection {
		setupLog.Info("Leader election enabled",
			cleanerlog.String("electionID", *leaderElectionID),
			cleanerlog.String("namespace", *leaderElectionNamespace))
	} else {
		setupLog.Warn("Leader election disabled - only safe for single replica")
	}
}

// newHealthMux builds the standalone health mux serving /healthz, /readyz,
// /startup and /leaderz.
func newHealthMux(healthChecker *controller.HealthChecker, leaderState *controller.LeaderState) *http.ServeMux {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := healthChecker.LivenessCheck(r); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	// /readyz serves the LEADERSHIP-AWARE readiness law (§11-§12 plus the
	// 2026-10-29 API-reachability failure class). The chart readiness probe
	// points here; probing /healthz (liveness) instead is what left the
	// enforcement-failure mode of 0.1.2 reporting Ready while dead.
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := healthChecker.LeadershipAwareReadinessCheck(leaderState)(r); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	healthMux.HandleFunc("/startup", func(w http.ResponseWriter, r *http.Request) {
		if err := healthChecker.StartupCheck(r); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	healthMux.Handle("/leaderz", controller.LeaderzCheck(leaderState))
	return healthMux
}

// initTracing initializes OTel tracing (non-fatal on failure) and returns
// the shutdown function, or nil when tracing init failed.
func initTracing(ctx context.Context) func() {
	traceShutdown, traceErr := observability.InitTracing(ctx)
	if traceErr != nil {
		setupLog.Warn("Tracing init failed (non-fatal)", cleanerlog.Error(traceErr))
		return nil
	}
	return func() {
		if sdErr := traceShutdown(context.Background()); sdErr != nil {
			setupLog.Warn("Tracing shutdown error", cleanerlog.Error(sdErr))
		}
	}
}

// startWebhookServer starts the admission webhook server on this replica
// (invoked for ALL replicas, before leader election). TLS is required
// unless --insecure-webhook is explicitly set (testing only).
func startWebhookServer(ctx context.Context, targetGate gcwebhook.TargetGate) {
	if !*enableWebhook {
		return
	}
	webhookServer, err := gcwebhook.NewWebhookServer(*webhookAddr, *webhookCertFile, *webhookKeyFile)
	if err != nil {
		setupLog.Error(err, "Error creating webhook server", cleanerlog.ErrorCode("WEBHOOK_CREATE_ERROR"))
		os.Exit(1)
	}
	webhookServer.SetTargetGate(targetGate)

	certExists := false
	keyExists := false
	if _, err := os.Stat(*webhookCertFile); err == nil {
		certExists = true
	}
	if _, err := os.Stat(*webhookKeyFile); err == nil {
		keyExists = true
	}

	if !certExists || !keyExists {
		if !*insecureWebhook {
			setupLog.Error(fmt.Errorf("%w (cert: %s, key: %s). TLS is required for production. Use --insecure-webhook flag only for testing", ErrWebhookTLSCertificatesMissing, *webhookCertFile, *webhookKeyFile), "TLS certificates missing", cleanerlog.ErrorCode("TLS_CERT_MISSING"))
			os.Exit(1)
		}
		setupLog.Warn("Webhook starting without TLS (insecure mode) - NOT RECOMMENDED FOR PRODUCTION", cleanerlog.Component("webhook"))
		go func() {
			if err := webhookServer.Start(ctx); err != nil {
				setupLog.Error(err, "Error starting webhook server", cleanerlog.ErrorCode("WEBHOOK_START_ERROR"))
			}
		}()
		return
	}

	go func() {
		if err := webhookServer.StartTLS(ctx, *webhookCertFile, *webhookKeyFile); err != nil {
			setupLog.Error(err, "Error starting webhook server", cleanerlog.ErrorCode("WEBHOOK_START_ERROR"))
		}
	}()
	setupLog.Info("Webhook server starting with TLS", cleanerlog.String("address", *webhookAddr), cleanerlog.Component("webhook"))
}

// runController runs the controller manager and all components (leader only
// — invoked by the election callback). The reconciler and health checker are
// created in runMain so health endpoints exist on standby replicas too
// (SUPPORT2-003 §11).
func runController(ctx context.Context, restCfg *rest.Config, mgrOpts *ctrl.Options, reconciler *controller.PolicyReconciler, healthChecker *controller.HealthChecker, scheme *runtime.Scheme, dynamicClient dynamic.Interface, statusUpdater *controller.StatusUpdater, eventRecorder *controller.EventRecorder, controllerConfig *config.ControllerConfig) {
	setupLog := logger.WithComponent("controller")

	// SUPPORT2-003 §11: the manager does not bind its own health port — the
	// standalone always-on server (runMain) owns it for leader AND standby.
	// mgrOpts.HealthProbeBindAddress is already "0" from baseOpts.
	mgr, err := ctrl.NewManager(restCfg, *mgrOpts)
	if err != nil {
		setupLog.Error(err, "Error creating controller manager", cleanerlog.ErrorCode("MANAGER_CREATE_ERROR"))
		os.Exit(1)
	}

	// Rebind the reconciler's client to the manager's cached client so
	// policy watch/list flows through the shared cache as before.
	reconciler.Client = mgr.GetClient()
	reconciler.Scheme = mgr.GetScheme()

	// Setup reconciler with manager
	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Error setting up reconciler", cleanerlog.ErrorCode("RECONCILER_SETUP_ERROR"))
		os.Exit(1)
	}

	// Create health checker for enhanced health checks (already created above)

	// Add enhanced liveness check (verifies active processing)
	if err := mgr.AddHealthzCheck("healthz", healthChecker.LivenessCheck); err != nil {
		setupLog.Error(err, "Error adding health check", cleanerlog.ErrorCode("HEALTH_CHECK_ERROR"))
		os.Exit(1)
	}

	// Add enhanced readiness check (verifies informer sync status)
	if err := mgr.AddReadyzCheck("readyz", healthChecker.ReadinessCheck); err != nil {
		setupLog.Error(err, "Error adding readiness check", cleanerlog.ErrorCode("READY_CHECK_ERROR"))
		os.Exit(1)
	}

	// Add startup check (simple initialization check)
	if err := mgr.AddHealthzCheck("startup", healthChecker.StartupCheck); err != nil {
		setupLog.Error(err, "Error adding startup check", cleanerlog.ErrorCode("STARTUP_CHECK_ERROR"))
		os.Exit(1)
	}

	// Start webhook server if enabled (separate from controller-runtime webhook server)
	// SUPPORT2-032R: the admission webhook is started in runMain for ALL
	// replicas (leader and standby) — see startWebhookServer. Inside the
	// leader-only callback the webhook Service routed admission dials to
	// the standby too, which serves nothing on :9443, so roughly half of
	// all admissions failed with 502 under failurePolicy=Fail.

	// Start the manager (this blocks until context is canceled)
	// mgr.Start() errors are typically non-fatal (e.g., context canceled on shutdown)
	// We don't call os.Exit here to allow graceful shutdown via defer cancel()
	setupLog.Info("Starting Zen Cleaner controller manager", cleanerlog.Operation("start"))
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "Error starting manager", cleanerlog.ErrorCode("MANAGER_START_ERROR"))
		// Don't call os.Exit here - let the defer cancel() run for cleanup
		return
	}

	setupLog.Info("Zen Cleaner controller shutdown complete", cleanerlog.Operation("shutdown"))
}
