package config

import (
	"os"
	"testing"
)

type mockCRDRateLimitConfig struct {
	rpm   int
	burst int
}

func (m *mockCRDRateLimitConfig) GetRequestsPerMinute() int {
	return m.rpm
}

func (m *mockCRDRateLimitConfig) GetBurst() int {
	return m.burst
}

func TestLoadIngesterRateLimitConfig(t *testing.T) {
	os.Unsetenv("RATE_LIMIT_INGESTER_TEST_RPM")
	os.Unsetenv("RATE_LIMIT_INGESTER_TEST_BURST")
	os.Unsetenv("RATE_LIMIT_INGESTER_DEFAULT_RPM")
	os.Unsetenv("RATE_LIMIT_INGESTER_DEFAULT_BURST")

	// No config should return nil
	cfg := LoadIngesterRateLimitConfig("test", nil)
	if cfg != nil {
		t.Error("Expected nil config when no env vars set")
	}

	os.Setenv("RATE_LIMIT_INGESTER_TEST_RPM", "1000")
	os.Setenv("RATE_LIMIT_INGESTER_TEST_BURST", "100")
	cfg = LoadIngesterRateLimitConfig("test", nil)
	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}
	if cfg.RequestsPerMinute != 1000 {
		t.Errorf("Expected 1000, got %d", cfg.RequestsPerMinute)
	}

	os.Unsetenv("RATE_LIMIT_INGESTER_TEST_RPM")
	os.Unsetenv("RATE_LIMIT_INGESTER_TEST_BURST")
}

func TestLoadIngesterRateLimitConfigWithCRD(t *testing.T) {
	os.Unsetenv("RATE_LIMIT_INGESTER_TEST_RPM")
	os.Unsetenv("RATE_LIMIT_INGESTER_TEST_BURST")

	mockCRD := &mockCRDRateLimitConfig{rpm: 500, burst: 50}
	cfg := LoadIngesterRateLimitConfig("test", mockCRD)
	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}
	if cfg.RequestsPerMinute != 500 {
		t.Errorf("Expected 500, got %d", cfg.RequestsPerMinute)
	}
	if cfg.Burst != 50 {
		t.Errorf("Expected 50, got %d", cfg.Burst)
	}
}

func TestGetIngesterRateLimitFromEnv(t *testing.T) {
	os.Unsetenv("RATE_LIMIT_INGESTER_TEST_REQUESTS_PER_MINUTE")
	val := getIngesterRateLimitFromEnv("test", "REQUESTS_PER_MINUTE")
	if val != 0 {
		t.Errorf("Expected 0 for missing env, got %d", val)
	}

	os.Setenv("RATE_LIMIT_INGESTER_TEST_REQUESTS_PER_MINUTE", "500")
	val = getIngesterRateLimitFromEnv("test", "REQUESTS_PER_MINUTE")
	if val != 500 {
		t.Errorf("Expected 500, got %d", val)
	}

	os.Setenv("RATE_LIMIT_INGESTER_TEST_REQUESTS_PER_MINUTE", "invalid")
	val = getIngesterRateLimitFromEnv("test", "REQUESTS_PER_MINUTE")
	if val != 0 {
		t.Errorf("Expected 0 for invalid value, got %d", val)
	}

	os.Unsetenv("RATE_LIMIT_INGESTER_TEST_REQUESTS_PER_MINUTE")
}

func TestSourceConfigRateLimitAdapter(t *testing.T) {
	adapter := NewSourceConfigRateLimitAdapter(600, 60)

	rpm := adapter.GetRequestsPerMinute()
	if rpm != 600 {
		t.Errorf("Expected 600, got %d", rpm)
	}

	burst := adapter.GetBurst()
	if burst != 60 {
		t.Errorf("Expected 60, got %d", burst)
	}
}
