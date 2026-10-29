/*
Copyright 2026 Zen-Mesh Contributors

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

// Package election provides simple leader election using client-go.
package election

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
)

// DefaultElectionID is the default leader election lock identity when none is set.
const DefaultElectionID = "zen-cleaner-leader-election"

// Config holds client-go leader election parameters.
type Config struct {
	ElectionID    string
	Namespace     string
	LeaseName     string
	LeaseDuration time.Duration
	RenewDeadline time.Duration
	RetryPeriod   time.Duration
	Enable        bool
	GetIdentity   func() string

	// OnLeadingChange is invoked on leadership transitions (SUPPORT2-003
	// §11: feeds the leadership-aware health server). Optional.
	OnLeadingChange func(leading bool)

	// OnAPIObservation is invoked after every leader-election lock
	// operation with the resulting reachability verdict (2026-10-29
	// launch-validation law): reachable=true means the lease was read or
	// written; reachable=false carries the terminal API error (dial
	// refused, RBAC denial, ...). This is the failure-observability seam
	// for cases like NetworkPolicy-enforced API egress rejection, where
	// client-go otherwise only logs and the pod reports Ready. Optional.
	OnAPIObservation func(reachable bool, err error)
}

// LeaderElector is implemented by types that run leader election until cancel.
type LeaderElector interface {
	Run(ctx context.Context) error
}

// leaderElectorAdapter wraps leaderelection.LeaderElector.
type leaderElectorAdapter struct {
	le *leaderelection.LeaderElector
}

func (a *leaderElectorAdapter) Run(ctx context.Context) error {
	a.le.Run(ctx)
	return nil // RunOrDie doesn't return
}

// NewLeaderElector creates a leader elector from the given config.
func NewLeaderElector(client kubernetes.Interface, cfg *Config) (LeaderElector, error) {
	lock := newLeaseLock(client, cfg)
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: cfg.LeaseDuration,
		RenewDeadline: cfg.RenewDeadline,
		RetryPeriod:   cfg.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {},
			OnStoppedLeading: func() {},
			OnNewLeader:      func(identity string) {},
		},
		WatchDog: leaderelection.NewLeaderHealthzAdaptor(0),
		Name:     cfg.ElectionID,
	})
	if err != nil {
		return nil, err
	}
	return &leaderElectorAdapter{le: le}, nil
}

func newLeaseLock(client kubernetes.Interface, cfg *Config) resourcelock.Interface {
	identity := cfg.GetIdentity
	if identity == nil {
		identity = getPodName
	}
	return &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.LeaseName,
			Namespace: cfg.Namespace,
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: identity(),
		},
	}
}

// ApplyDefaults fills in missing fields on cfg (nil-safe).
func ApplyDefaults(cfg *Config) *Config {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	}
	if cfg.ElectionID == "" {
		cfg.ElectionID = DefaultElectionID
	}
	if cfg.LeaseName == "" {
		cfg.LeaseName = cfg.ElectionID
	}
	if cfg.LeaseDuration == 0 {
		cfg.LeaseDuration = 15 * time.Second
	}
	if cfg.RenewDeadline == 0 {
		cfg.RenewDeadline = 10 * time.Second
	}
	if cfg.RetryPeriod == 0 {
		cfg.RetryPeriod = 5 * time.Second
	}
	return cfg
}

// Runner wires callbacks around a LeaderElector.
type Runner struct {
	Elector          LeaderElector
	OnStartedLeading func(ctx context.Context)
	OnStoppedLeading func()
	OnNewLeader      func(identity string)
	ElectionID       string
}

// Run starts the configured elector (no-op if missing).
func (r *Runner) Run(ctx context.Context) error {
	if r.Elector == nil {
		return nil // Should not happen if configured correctly
	}

	return r.Elector.Run(ctx)
}

// NewRunner builds a Runner with the given elector and callbacks.
func NewRunner(elector LeaderElector, onStartedLeading func(context.Context), onStoppedLeading func(), onNewLeader func(string), electionID string) *Runner {
	return &Runner{
		Elector:          elector,
		OnStartedLeading: onStartedLeading,
		OnStoppedLeading: onStoppedLeading,
		OnNewLeader:      onNewLeader,
		ElectionID:       electionID,
	}
}

// RunWithLeaderElection runs runFn on the leader using client-go election.
// It is the main entry point for this package.
func RunWithLeaderElection(ctx context.Context, cfg *Config, client kubernetes.Interface, runFn func(context.Context)) error {
	if cfg == nil {
		cfg = &Config{Enable: true}
	}

	if !cfg.Enable {
		klog.InfoS("Leader election disabled, running directly")
		runFn(ctx)
		return nil
	}

	ApplyDefaults(cfg)

	identity := cfg.GetIdentity
	if identity == nil {
		identity = getPodName
	}

	var lock resourcelock.Interface = &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.LeaseName,
			Namespace: cfg.Namespace,
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: identity(),
		},
	}
	if cfg.OnAPIObservation != nil {
		lock = &observedLock{delegate: lock, onObservation: cfg.OnAPIObservation}
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: cfg.LeaseDuration,
		RenewDeadline: cfg.RenewDeadline,
		RetryPeriod:   cfg.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				klog.InfoS("Started leading", "electionID", cfg.ElectionID)
				if cfg.OnLeadingChange != nil {
					cfg.OnLeadingChange(true)
				}
				runFn(ctx)
			},
			OnStoppedLeading: func() {
				klog.InfoS("Stopped leading, shutting down")
				if cfg.OnLeadingChange != nil {
					cfg.OnLeadingChange(false)
				}
			},
			OnNewLeader: func(id string) {
				if id != identity() {
					klog.InfoS("New leader elected", "leader", id)
				}
			},
		},
		WatchDog: leaderelection.NewLeaderHealthzAdaptor(0),
		Name:     cfg.ElectionID,
	})

	return nil
}

func getPodName() string {
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return "unknown"
}

// ShutdownContext returns a context that is canceled on SIGINT or SIGTERM.
func ShutdownContext(ctx context.Context, name string) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
}

// observedLock wraps a resourcelock.Interface and reports every Get/Update
// outcome to OnAPIObservation. Leader-election failures are the earliest and
// most precise signal of API unreachability (connection refused under
// NetworkPolicy enforcement, RBAC denial, ...); client-go only klog-logs
// them, so without this seam the pod reports Ready while dead.
type observedLock struct {
	delegate      resourcelock.Interface
	onObservation func(reachable bool, err error)
}

func (l *observedLock) report(err error) {
	if l.onObservation == nil {
		return
	}
	// Reachability semantics (2026-10-29): "reachable" means the API SERVER
	// RESPONDED. Object-level API errors (NotFound on the first lease GET of
	// a fresh cluster, 401/403, conflict, ...) prove reachability — treating
	// them as unreachable produced a false APIUnreachable event pair at
	// every cold start. Only transport/unknown failures mean unreachable.
	if err == nil {
		l.onObservation(true, nil)
		return
	}
	if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) ||
		apierrors.IsUnauthorized(err) || apierrors.IsInvalid(err) ||
		apierrors.IsConflict(err) || apierrors.IsGone(err) ||
		apierrors.IsMethodNotSupported(err) || apierrors.IsAlreadyExists(err) {
		l.onObservation(true, err)
		return
	}
	l.onObservation(false, err)
}

func (l *observedLock) Get(ctx context.Context) (*resourcelock.LeaderElectionRecord, []byte, error) {
	record, raw, err := l.delegate.Get(ctx)
	l.report(err)
	return record, raw, err
}

func (l *observedLock) Create(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	err := l.delegate.Create(ctx, ler)
	l.report(err)
	return err
}

func (l *observedLock) Update(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	err := l.delegate.Update(ctx, ler)
	l.report(err)
	return err
}

func (l *observedLock) RecordEvent(s string) { l.delegate.RecordEvent(s) }

func (l *observedLock) Describe() string { return l.delegate.Describe() }

func (l *observedLock) Identity() string { return l.delegate.Identity() }
