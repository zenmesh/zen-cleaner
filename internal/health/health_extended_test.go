package health

import (
	"net/http"
	"testing"
	"time"
)

func TestInformerSyncCheckerWithNoInformers(t *testing.T) {
	checker := NewInformerSyncChecker(func() map[string]func() bool {
		return map[string]func() bool{}
	})

	err := checker.ReadinessCheck(nil)
	if err != nil {
		t.Errorf("Expected no error with no informers, got %v", err)
	}
}

func TestInformerSyncCheckerWithSyncedInformers(t *testing.T) {
	synced := true
	checker := NewInformerSyncChecker(func() map[string]func() bool {
		return map[string]func() bool{
			"informer1": func() bool { return synced },
			"informer2": func() bool { return synced },
		}
	})

	err := checker.ReadinessCheck(nil)
	if err != nil {
		t.Errorf("Expected no error with synced informers, got %v", err)
	}
}

func TestInformerSyncCheckerWithUnsyncedInformers(t *testing.T) {
	checker := NewInformerSyncChecker(func() map[string]func() bool {
		return map[string]func() bool{
			"informer1": func() bool { return true },
			"informer2": func() bool { return false },
		}
	})

	err := checker.ReadinessCheck(nil)
	if err == nil {
		t.Error("Expected error with unsynced informers")
	}
}

func TestInformerSyncCheckerNilGetter(t *testing.T) {
	checker := NewInformerSyncChecker(nil)

	err := checker.ReadinessCheck(nil)
	if err == nil {
		t.Error("Expected error with nil getter")
	}
}

func TestInformerSyncCheckerLivenessCheck(t *testing.T) {
	checker := NewInformerSyncChecker(func() map[string]func() bool {
		return map[string]func() bool{
			"informer1": func() bool { return true },
		}
	})

	err := checker.LivenessCheck(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestInformerSyncCheckerStartupCheck(t *testing.T) {
	checker := NewInformerSyncChecker(func() map[string]func() bool {
		return map[string]func() bool{}
	})

	err := checker.StartupCheck(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestActivityCheckerRecentlyActive(t *testing.T) {
	checker := NewActivityChecker(time.Now, 5*time.Minute)

	err := checker.ReadinessCheck(nil)
	if err != nil {
		t.Errorf("Expected no error when recently active, got %v", err)
	}
}

func TestActivityCheckerInactiveTooLong(t *testing.T) {
	checker := NewActivityChecker(func() time.Time {
		return time.Now().Add(-10 * time.Minute)
	}, 5*time.Minute)

	err := checker.ReadinessCheck(nil)
	if err == nil {
		t.Error("Expected error when inactive too long")
	}
}

func TestActivityCheckerLivenessCheck(t *testing.T) {
	checker := NewActivityChecker(time.Now, 5*time.Minute)

	err := checker.LivenessCheck(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestActivityCheckerStartupCheck(t *testing.T) {
	checker := NewActivityChecker(time.Now, 5*time.Minute)

	err := checker.StartupCheck(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestCompositeCheckerAllPass(t *testing.T) {
	checker1 := &mockChecker{err: nil}
	checker2 := &mockChecker{err: nil}
	composite := NewCompositeChecker(checker1, checker2)

	err := composite.ReadinessCheck(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestCompositeCheckerOneFails(t *testing.T) {
	checker1 := &mockChecker{err: nil}
	checker2 := &mockChecker{err: ErrNotReady}
	composite := NewCompositeChecker(checker1, checker2)

	err := composite.ReadinessCheck(nil)
	if err == nil {
		t.Error("Expected error when one checker fails")
	}
}

func TestCompositeCheckerAddChecker(t *testing.T) {
	composite := NewCompositeChecker()
	composite.AddChecker(&mockChecker{err: nil})

	err := composite.ReadinessCheck(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestCompositeCheckerLivenessCheck(t *testing.T) {
	composite := NewCompositeChecker(&mockChecker{err: nil})

	err := composite.LivenessCheck(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestCompositeCheckerStartupCheck(t *testing.T) {
	composite := NewCompositeChecker(&mockChecker{err: nil})

	err := composite.StartupCheck(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

type mockChecker struct {
	err error
}

func (m *mockChecker) ReadinessCheck(req *http.Request) error {
	return m.err
}

func (m *mockChecker) LivenessCheck(req *http.Request) error {
	return m.err
}

func (m *mockChecker) StartupCheck(req *http.Request) error {
	return m.err
}
