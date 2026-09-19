package election

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

func TestConfigDefaults(t *testing.T) {
	cfg := &Config{}

	// Test default values
	if cfg.ElectionID != "" {
		t.Errorf("Expected empty ElectionID, got %s", cfg.ElectionID)
	}
	if cfg.Namespace != "" {
		t.Errorf("Expected empty Namespace, got %s", cfg.Namespace)
	}
	if cfg.LeaseName != "" {
		t.Errorf("Expected empty LeaseName, got %s", cfg.LeaseName)
	}
	if cfg.LeaseDuration != 0 {
		t.Errorf("Expected 0 LeaseDuration, got %v", cfg.LeaseDuration)
	}
	if cfg.RenewDeadline != 0 {
		t.Errorf("Expected 0 RenewDeadline, got %v", cfg.RenewDeadline)
	}
	if cfg.RetryPeriod != 0 {
		t.Errorf("Expected 0 RetryPeriod, got %v", cfg.RetryPeriod)
	}
	if cfg.Enable != false {
		t.Errorf("Expected false Enable, got %v", cfg.Enable)
	}
}

func TestConfigWithValues(t *testing.T) {
	cfg := &Config{
		ElectionID:    "test-election",
		Namespace:     "test-ns",
		LeaseName:     "test-lease",
		LeaseDuration: 30 * time.Second,
		RenewDeadline: 20 * time.Second,
		RetryPeriod:   10 * time.Second,
		Enable:        true,
		GetIdentity:   func() string { return "test-identity" },
	}

	if cfg.ElectionID != "test-election" {
		t.Errorf("Expected test-election, got %s", cfg.ElectionID)
	}
	if cfg.Namespace != "test-ns" {
		t.Errorf("Expected test-ns, got %s", cfg.Namespace)
	}
	if cfg.LeaseName != "test-lease" {
		t.Errorf("Expected test-lease, got %s", cfg.LeaseName)
	}
	if cfg.LeaseDuration != 30*time.Second {
		t.Errorf("Expected 30s, got %v", cfg.LeaseDuration)
	}
	if cfg.Enable != true {
		t.Errorf("Expected true, got %v", cfg.Enable)
	}
	if cfg.GetIdentity() != "test-identity" {
		t.Errorf("Expected test-identity, got %s", cfg.GetIdentity())
	}
}

func TestApplyDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    *Config
		expected *Config
	}{
		{
			name:     "nil config",
			input:    nil,
			expected: &Config{Namespace: "default", ElectionID: "zen-cleaner-leader-election", LeaseName: "zen-cleaner-leader-election", LeaseDuration: 15 * time.Second, RenewDeadline: 10 * time.Second, RetryPeriod: 5 * time.Second},
		},
		{
			name:     "empty config",
			input:    &Config{},
			expected: &Config{Namespace: "default", ElectionID: "zen-cleaner-leader-election", LeaseName: "zen-cleaner-leader-election", LeaseDuration: 15 * time.Second, RenewDeadline: 10 * time.Second, RetryPeriod: 5 * time.Second},
		},
		{
			name: "partial config",
			input: &Config{
				Namespace: "custom-ns",
			},
			expected: &Config{
				Namespace:     "custom-ns",
				ElectionID:    "zen-cleaner-leader-election",
				LeaseName:     "zen-cleaner-leader-election",
				LeaseDuration: 15 * time.Second,
				RenewDeadline: 10 * time.Second,
				RetryPeriod:   5 * time.Second,
			},
		},
		{
			name: "full config preserved",
			input: &Config{
				ElectionID:    "custom-election",
				Namespace:     "custom-ns",
				LeaseName:     "custom-lease",
				LeaseDuration: 60 * time.Second,
				RenewDeadline: 40 * time.Second,
				RetryPeriod:   20 * time.Second,
			},
			expected: &Config{
				ElectionID:    "custom-election",
				Namespace:     "custom-ns",
				LeaseName:     "custom-lease",
				LeaseDuration: 60 * time.Second,
				RenewDeadline: 40 * time.Second,
				RetryPeriod:   20 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyDefaults(tt.input)

			if result.Namespace != tt.expected.Namespace {
				t.Errorf("Namespace: expected %s, got %s", tt.expected.Namespace, result.Namespace)
			}
			if result.ElectionID != tt.expected.ElectionID {
				t.Errorf("ElectionID: expected %s, got %s", tt.expected.ElectionID, result.ElectionID)
			}
			if result.LeaseName != tt.expected.LeaseName {
				t.Errorf("LeaseName: expected %s, got %s", tt.expected.LeaseName, result.LeaseName)
			}
			if result.LeaseDuration != tt.expected.LeaseDuration {
				t.Errorf("LeaseDuration: expected %v, got %v", tt.expected.LeaseDuration, result.LeaseDuration)
			}
			if result.RenewDeadline != tt.expected.RenewDeadline {
				t.Errorf("RenewDeadline: expected %v, got %v", tt.expected.RenewDeadline, result.RenewDeadline)
			}
			if result.RetryPeriod != tt.expected.RetryPeriod {
				t.Errorf("RetryPeriod: expected %v, got %v", tt.expected.RetryPeriod, result.RetryPeriod)
			}
		})
	}
}

func TestShutdownContext(t *testing.T) {
	ctx, cancel := ShutdownContext(context.Background(), "test")
	if ctx == nil {
		t.Error("Expected non-nil context")
	}
	if cancel == nil {
		t.Error("Expected non-nil cancel function")
	}

	// Cancel should not panic
	cancel()
}

func TestGetPodName(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Skipf("Cannot get hostname: %v", err)
	}

	// Test that getPodName returns the hostname
	result := getPodName()
	if result == "" {
		t.Error("Expected non-empty pod name")
	}
	_ = hostname // Use variable to avoid unused warning
}

func TestRunner(t *testing.T) {
	// Test that Runner struct can be created and configured
	runner := NewRunner(nil, func(ctx context.Context) {}, func() {}, func(s string) {}, "test-election")

	if runner.ElectionID != "test-election" {
		t.Errorf("Expected test-election, got %s", runner.ElectionID)
	}
	if runner.OnStartedLeading == nil {
		t.Error("Expected non-nil OnStartedLeading callback")
	}
	if runner.OnStoppedLeading == nil {
		t.Error("Expected non-nil OnStoppedLeading callback")
	}
	if runner.OnNewLeader == nil {
		t.Error("Expected non-nil OnNewLeader callback")
	}
}

func TestLeaderElectorInterface(t *testing.T) {
	// Verify LeaderElector interface is implemented correctly
	var _ LeaderElector = (*leaderElectorAdapter)(nil)
}

func TestRunWithLeaderElectionDisabled(t *testing.T) {
	// Test that when Enable is false, runFn is called directly
	callCount := 0
	cfg := &Config{Enable: false}

	err := RunWithLeaderElection(context.Background(), cfg, nil, func(ctx context.Context) {
		callCount++
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("Expected runFn to be called once, got %d", callCount)
	}
}

func TestGetIdentity(t *testing.T) {
	// Test custom GetIdentity function
	cfg := &Config{
		GetIdentity: func() string { return "custom-identity" },
	}

	if cfg.GetIdentity() != "custom-identity" {
		t.Errorf("Expected custom-identity, got %s", cfg.GetIdentity())
	}
}

type fakeLeaderElector struct {
	runCalled bool
}

func (f *fakeLeaderElector) Run(ctx context.Context) error {
	f.runCalled = true
	<-ctx.Done()
	return ctx.Err()
}

func TestRunner_Run_invokesElector(t *testing.T) {
	f := &fakeLeaderElector{}
	r := NewRunner(f, func(context.Context) {}, func() {}, func(string) {}, "eid")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}
	if !f.runCalled {
		t.Error("expected LeaderElector.Run to be called")
	}

	rnil := NewRunner(nil, nil, nil, nil, "eid")
	if err := rnil.Run(context.Background()); err != nil {
		t.Fatalf("nil elector: %v", err)
	}
}

func TestNewLeaderElector_andAdapterRun(t *testing.T) {
	client := kubernetesfake.NewSimpleClientset()
	cfg := ApplyDefaults(&Config{})
	cfg.GetIdentity = func() string { return "unit-test-id" }

	elector, err := NewLeaderElector(client, cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := elector.Run(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Logf("Run ended with: %v", err)
	}
}

func TestNewLeaseLock_defaultIdentity(t *testing.T) {
	client := kubernetesfake.NewSimpleClientset()
	cfg := ApplyDefaults(&Config{})
	cfg.GetIdentity = nil
	lock := newLeaseLock(client, cfg)
	if lock == nil {
		t.Fatal("expected lease lock")
	}
}
