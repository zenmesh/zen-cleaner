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
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/zenmesh/zen-cleaner/internal/election"
	sdklog "github.com/zenmesh/zen-cleaner/internal/logging"
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
	logger    *sdklog.Logger
	setupLog  *sdklog.Logger
)

var (
	metricsAddr              = flag.String("metrics-addr", ":8080", "The address the metric endpoint binds to")
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

func runMain() int {
	// Initialize the zen-cleaner logger (configures the controller-runtime logger automatically)
	logger = sdklog.NewLogger("zen-cleaner")
	setupLog = logger.WithComponent("setup")
	setupLog.Debug("Zen Cleaner controller starting", sdklog.String("version", version), sdklog.String("commit", commit), sdklog.String("buildDate", buildDate))

	// OpenTelemetry tracing initialization can be added here when a tracing backend is configured
	// For now, continue without tracing

	// Get config using controller-runtime (handles kubeconfig flag automatically)
	restCfg := ctrl.GetConfigOrDie()

	// Create dynamic client (still needed for resource informers)
	dynamicClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		setupLog.Error(err, "Error building dynamic client", sdklog.ErrorCode("CLIENT_ERROR"))
		return 1
	}

	// Create Kubernetes client for events
	kubeClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		setupLog.Error(err, "Error building Kubernetes client", sdklog.ErrorCode("CLIENT_ERROR"))
		return 1
	}

	// Create scheme and add ZenCleanerPolicy types
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "Error adding scheme", sdklog.ErrorCode("SCHEME_ERROR"))
		return 1
	}

	// Load controller configuration
	controllerConfig := config.NewControllerConfig()
	if err := controllerConfig.LoadFromEnv(); err != nil {
		setupLog.Error(err, "Error loading configuration from environment", sdklog.ErrorCode("CONFIG_LOAD_ERROR"))
		return 1
	}
	// Precedence: env (ZEN_CLEANER_*) overrides defaults; explicit flags
	// override env. Flags not explicitly set must not clobber env-provided
	// values, otherwise the documented ZEN_CLEANER_* contract is inert
	// whenever a deployment passes the flag defaults verbatim (SUPPORT2-002).
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

	setupLog.Info("Controller configuration",
		sdklog.String("cleanupInterval", controllerConfig.CleanupInterval.String()),
		sdklog.Int("maxDeletionsPerSecond", controllerConfig.MaxDeletionsPerSecond),
		sdklog.Int("batchSize", controllerConfig.BatchSize),
		sdklog.Int("maxConcurrentEvaluations", controllerConfig.MaxConcurrentEvaluations))

	// Create status updater with configuration
	statusUpdater := controller.NewStatusUpdaterWithConfig(dynamicClient, controllerConfig)

	// Create event recorder
	eventRecorder := controller.NewEventRecorder(kubeClient)

	// Setup controller-runtime manager
	baseOpts := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: *metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    9443,
			CertDir: "", // We'll handle webhook separately for now
		}),
		HealthProbeBindAddress: ":8081", // Health probes on separate port (controller-runtime requirement)
	}

	// Configure manager options (no leader election - we use client-go leader election)
	mgrOpts := baseOpts

	// Set up graceful shutdown context
	ctx, cancel := election.ShutdownContext(context.Background(), "zen-cleaner")
	defer cancel()

	// Run with leader election using client-go
	leConfig := &election.Config{
		ElectionID: *leaderElectionID,
		Namespace:  *leaderElectionNamespace,
		Enable:     *leaderElection,
	}

	if *leaderElection {
		setupLog.Info("Leader election enabled",
			sdklog.String("electionID", *leaderElectionID),
			sdklog.String("namespace", *leaderElectionNamespace))
	} else {
		setupLog.Warn("Leader election disabled - only safe for single replica")
	}

	err = election.RunWithLeaderElection(ctx, leConfig, kubeClient, func(runCtx context.Context) {
		runController(runCtx, restCfg, &mgrOpts, scheme, dynamicClient, statusUpdater, eventRecorder, controllerConfig)
	})
	if err != nil {
		setupLog.Error(err, "Leader election failed", sdklog.ErrorCode("LEADER_ELECTION_ERROR"))
		return 1
	}
	return 0
}

// runController runs the controller manager and all components.
func runController(ctx context.Context, restCfg *rest.Config, mgrOpts *ctrl.Options, scheme *runtime.Scheme, dynamicClient dynamic.Interface, statusUpdater *controller.StatusUpdater, eventRecorder *controller.EventRecorder, controllerConfig *config.ControllerConfig) {
	setupLog := logger.WithComponent("controller")

	mgr, err := ctrl.NewManager(restCfg, *mgrOpts)
	if err != nil {
		setupLog.Error(err, "Error creating controller manager", sdklog.ErrorCode("MANAGER_CREATE_ERROR"))
		os.Exit(1)
	}

	// Create policy reconciler with RESTMapper (leader election handled by controller-runtime Manager)
	// RESTMapper enables reliable GVR resolution for irregular CRDs
	reconciler := controller.NewPolicyReconcilerWithRESTMapper(
		mgr.GetClient(),
		mgr.GetScheme(),
		dynamicClient,
		mgr.GetRESTMapper(),
		statusUpdater,
		eventRecorder,
		controllerConfig,
	)

	// Create health checker with reconciler reference
	healthChecker := controller.NewHealthChecker(reconciler)

	// Setup reconciler with manager
	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Error setting up reconciler", sdklog.ErrorCode("RECONCILER_SETUP_ERROR"))
		os.Exit(1)
	}

	// Create health checker for enhanced health checks (already created above)

	// Add enhanced liveness check (verifies active processing)
	if err := mgr.AddHealthzCheck("healthz", healthChecker.LivenessCheck); err != nil {
		setupLog.Error(err, "Error adding health check", sdklog.ErrorCode("HEALTH_CHECK_ERROR"))
		os.Exit(1)
	}

	// Add enhanced readiness check (verifies informer sync status)
	if err := mgr.AddReadyzCheck("readyz", healthChecker.ReadinessCheck); err != nil {
		setupLog.Error(err, "Error adding readiness check", sdklog.ErrorCode("READY_CHECK_ERROR"))
		os.Exit(1)
	}

	// Add startup check (simple initialization check)
	if err := mgr.AddHealthzCheck("startup", healthChecker.StartupCheck); err != nil {
		setupLog.Error(err, "Error adding startup check", sdklog.ErrorCode("STARTUP_CHECK_ERROR"))
		os.Exit(1)
	}

	// Start webhook server if enabled (separate from controller-runtime webhook server)
	var webhookServer *gcwebhook.WebhookServer
	if *enableWebhook {
		var err error
		webhookServer, err = gcwebhook.NewWebhookServer(*webhookAddr, *webhookCertFile, *webhookKeyFile)
		if err != nil {
			setupLog.Error(err, "Error creating webhook server", sdklog.ErrorCode("WEBHOOK_CREATE_ERROR"))
			os.Exit(1)
		}

		// Check if TLS files exist
		certExists := false
		keyExists := false
		if _, err := os.Stat(*webhookCertFile); err == nil {
			certExists = true
		}
		if _, err := os.Stat(*webhookKeyFile); err == nil {
			keyExists = true
		}

		// TLS files missing - check if insecure mode is allowed (before creating context)
		if !certExists || !keyExists {
			if !*insecureWebhook {
				setupLog.Error(fmt.Errorf("%w (cert: %s, key: %s). TLS is required for production. Use --insecure-webhook flag only for testing", ErrWebhookTLSCertificatesMissing, *webhookCertFile, *webhookKeyFile), "TLS certificates missing", sdklog.ErrorCode("TLS_CERT_MISSING"))
				os.Exit(1)
			}
		}
	}

	// Graceful shutdown is handled by election context
	// The election.RunWithLeaderElection provides the context that cancels on SIGINT/SIGTERM

	// Start webhook server if enabled (now that context is created)
	if *enableWebhook {
		// Check if TLS files exist (already checked above, but need to check again for the actual start)
		certExists := false
		keyExists := false
		if _, err := os.Stat(*webhookCertFile); err == nil {
			certExists = true
		}
		if _, err := os.Stat(*webhookKeyFile); err == nil {
			keyExists = true
		}

		if certExists && keyExists {
			// TLS files exist, start with TLS
			go func() {
				if err := webhookServer.StartTLS(ctx, *webhookCertFile, *webhookKeyFile); err != nil {
					setupLog.Error(err, "Error starting webhook server", sdklog.ErrorCode("WEBHOOK_START_ERROR"))
				}
			}()
			setupLog.Info("Webhook server starting with TLS", sdklog.String("address", *webhookAddr), sdklog.Component("webhook"))
		} else {
			setupLog.Warn("Webhook starting without TLS (insecure mode) - NOT RECOMMENDED FOR PRODUCTION", sdklog.Component("webhook"))
			go func() {
				if err := webhookServer.Start(ctx); err != nil {
					setupLog.Error(err, "Error starting webhook server", sdklog.ErrorCode("WEBHOOK_START_ERROR"))
				}
			}()
		}
	}

	// Start the manager (this blocks until context is canceled)
	// mgr.Start() errors are typically non-fatal (e.g., context canceled on shutdown)
	// We don't call os.Exit here to allow graceful shutdown via defer cancel()
	setupLog.Info("Starting Zen Cleaner controller manager", sdklog.Operation("start"))
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "Error starting manager", sdklog.ErrorCode("MANAGER_START_ERROR"))
		// Don't call os.Exit here - let the defer cancel() run for cleanup
		return
	}

	setupLog.Info("Zen Cleaner controller shutdown complete", sdklog.Operation("shutdown"))
}
