package config

import (
	"os"
	"testing"
	"time"
)

func TestRequireEnv(t *testing.T) {
	os.Unsetenv("TEST_VAR")
	_, err := RequireEnv("TEST_VAR")
	if err == nil {
		t.Error("Expected error for missing env var")
	}

	os.Setenv("TEST_VAR", "value")
	val, err := RequireEnv("TEST_VAR")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != "value" {
		t.Errorf("Expected 'value', got '%s'", val)
	}
	os.Unsetenv("TEST_VAR")
}

func TestRequireEnvWithDefault(t *testing.T) {
	os.Unsetenv("TEST_DEFAULT")
	val := RequireEnvWithDefault("TEST_DEFAULT", "default")
	if val != "default" {
		t.Errorf("Expected 'default', got '%s'", val)
	}

	os.Setenv("TEST_DEFAULT", "actual")
	val = RequireEnvWithDefault("TEST_DEFAULT", "default")
	if val != "actual" {
		t.Errorf("Expected 'actual', got '%s'", val)
	}
	os.Unsetenv("TEST_DEFAULT")
}

func TestRequireEnvInt(t *testing.T) {
	os.Unsetenv("TEST_INT")
	_, err := RequireEnvInt("TEST_INT")
	if err == nil {
		t.Error("Expected error for missing env var")
	}

	os.Setenv("TEST_INT", "42")
	val, err := RequireEnvInt("TEST_INT")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("Expected 42, got %d", val)
	}
	os.Unsetenv("TEST_INT")

	os.Setenv("TEST_INT", "invalid")
	_, err = RequireEnvInt("TEST_INT")
	if err == nil {
		t.Error("Expected error for invalid int")
	}
	os.Unsetenv("TEST_INT")
}

func TestRequireEnvIntWithDefault(t *testing.T) {
	os.Unsetenv("TEST_INT_DFT")
	val := RequireEnvIntWithDefault("TEST_INT_DFT", 100)
	if val != 100 {
		t.Errorf("Expected 100, got %d", val)
	}

	os.Setenv("TEST_INT_DFT", "50")
	val = RequireEnvIntWithDefault("TEST_INT_DFT", 100)
	if val != 50 {
		t.Errorf("Expected 50, got %d", val)
	}
	os.Unsetenv("TEST_INT_DFT")
}

func TestRequireEnvBool(t *testing.T) {
	os.Unsetenv("TEST_BOOL")
	_, err := RequireEnvBool("TEST_BOOL")
	if err == nil {
		t.Error("Expected error for missing env var")
	}

	os.Setenv("TEST_BOOL", "true")
	val, err := RequireEnvBool("TEST_BOOL")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != true {
		t.Errorf("Expected true, got %v", val)
	}
	os.Unsetenv("TEST_BOOL")

	os.Setenv("TEST_BOOL", "false")
	val, err = RequireEnvBool("TEST_BOOL")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != false {
		t.Errorf("Expected false, got %v", val)
	}
	os.Unsetenv("TEST_BOOL")
}

func TestRequireEnvBoolWithDefault(t *testing.T) {
	os.Unsetenv("TEST_BOOL_DFT")
	val := RequireEnvBoolWithDefault("TEST_BOOL_DFT", true)
	if val != true {
		t.Errorf("Expected true, got %v", val)
	}

	os.Setenv("TEST_BOOL_DFT", "false")
	val = RequireEnvBoolWithDefault("TEST_BOOL_DFT", true)
	if val != false {
		t.Errorf("Expected false, got %v", val)
	}
	os.Unsetenv("TEST_BOOL_DFT")
}

func TestRequireEnvOneOf(t *testing.T) {
	os.Unsetenv("TEST_ONEOF")
	allowed := []string{"a", "b", "c"}
	_, err := RequireEnvOneOf("TEST_ONEOF", allowed)
	if err == nil {
		t.Error("Expected error for missing env var")
	}

	os.Setenv("TEST_ONEOF", "invalid")
	_, err = RequireEnvOneOf("TEST_ONEOF", allowed)
	if err == nil {
		t.Error("Expected error for invalid value")
	}
	os.Unsetenv("TEST_ONEOF")

	os.Setenv("TEST_ONEOF", "a")
	val, err := RequireEnvOneOf("TEST_ONEOF", allowed)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != "a" {
		t.Errorf("Expected 'a', got '%s'", val)
	}
	os.Unsetenv("TEST_ONEOF")
}

func TestRequireEnvSecret(t *testing.T) {
	os.Unsetenv("TEST_SECRET")
	_, err := RequireEnvSecret("TEST_SECRET", 8)
	if err == nil {
		t.Error("Expected error for missing env var")
	}

	os.Setenv("TEST_SECRET", "short")
	_, err = RequireEnvSecret("TEST_SECRET", 8)
	if err == nil {
		t.Error("Expected error for too short secret")
	}
	os.Unsetenv("TEST_SECRET")

	os.Setenv("TEST_SECRET", "long-enough-secret")
	val, err := RequireEnvSecret("TEST_SECRET", 8)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != "long-enough-secret" {
		t.Errorf("Unexpected value: %s", val)
	}
	os.Unsetenv("TEST_SECRET")
}

func TestRequireAtLeastOne(t *testing.T) {
	os.Unsetenv("TEST_KEY1")
	os.Unsetenv("TEST_KEY2")

	err := RequireAtLeastOne([]string{"TEST_KEY1", "TEST_KEY2"})
	if err == nil {
		t.Error("Expected error when all keys missing")
	}

	os.Setenv("TEST_KEY1", "value")
	err = RequireAtLeastOne([]string{"TEST_KEY1", "TEST_KEY2"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	os.Unsetenv("TEST_KEY1")
	os.Unsetenv("TEST_KEY2")
}

func TestValidateProduction(t *testing.T) {
	// This test just ensures the function doesn't panic
	// In production environment (when KUBERNETES_SERVICE_HOST is set), it should pass
	_ = ValidateProduction()
}

func TestRequireEnvDuration(t *testing.T) {
	os.Unsetenv("TEST_DURATION")
	_, err := RequireEnvDuration("TEST_DURATION")
	if err == nil {
		t.Error("Expected error for missing env var")
	}

	os.Setenv("TEST_DURATION", "30s")
	val, err := RequireEnvDuration("TEST_DURATION")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 30*time.Second {
		t.Errorf("Expected 30s, got %v", val)
	}
	os.Unsetenv("TEST_DURATION")

	os.Setenv("TEST_DURATION", "invalid")
	_, err = RequireEnvDuration("TEST_DURATION")
	if err == nil {
		t.Error("Expected error for invalid duration")
	}
	os.Unsetenv("TEST_DURATION")
}

func TestRequireEnvDurationWithDefault(t *testing.T) {
	os.Unsetenv("TEST_DURATION_DFT")
	val := RequireEnvDurationWithDefault("TEST_DURATION_DFT", 60*time.Second)
	if val != 60*time.Second {
		t.Errorf("Expected 60s, got %v", val)
	}

	os.Setenv("TEST_DURATION_DFT", "30s")
	val = RequireEnvDurationWithDefault("TEST_DURATION_DFT", 60*time.Second)
	if val != 30*time.Second {
		t.Errorf("Expected 30s, got %v", val)
	}
	os.Unsetenv("TEST_DURATION_DFT")
}

func TestRequireEnvCSV(t *testing.T) {
	os.Unsetenv("TEST_CSV")
	_, err := RequireEnvCSV("TEST_CSV")
	if err == nil {
		t.Error("Expected error for missing env var")
	}

	os.Setenv("TEST_CSV", "a,b,c")
	val, err := RequireEnvCSV("TEST_CSV")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(val) != 3 || val[0] != "a" || val[1] != "b" || val[2] != "c" {
		t.Errorf("Unexpected value: %v", val)
	}
	os.Unsetenv("TEST_CSV")
}

func TestRequireEnvCSVWithDefault(t *testing.T) {
	os.Unsetenv("TEST_CSV_DFT")
	val := RequireEnvCSVWithDefault("TEST_CSV_DFT", []string{"x", "y"})
	if len(val) != 2 || val[0] != "x" || val[1] != "y" {
		t.Errorf("Expected default, got %v", val)
	}

	os.Setenv("TEST_CSV_DFT", "a,b")
	val = RequireEnvCSVWithDefault("TEST_CSV_DFT", []string{"x", "y"})
	if len(val) != 2 || val[0] != "a" || val[1] != "b" {
		t.Errorf("Expected a,b, got %v", val)
	}
	os.Unsetenv("TEST_CSV_DFT")
}

func TestRequireEnvURL(t *testing.T) {
	os.Unsetenv("TEST_REQ_URL")
	_, err := RequireEnvURL("TEST_REQ_URL")
	if err == nil {
		t.Error("expected error when unset")
	}

	os.Setenv("TEST_REQ_URL", "ftp://bad")
	_, err = RequireEnvURL("TEST_REQ_URL")
	if err == nil {
		t.Error("expected error for non-http(s) scheme")
	}

	os.Setenv("TEST_REQ_URL", "https://example.com/path")
	val, err := RequireEnvURL("TEST_REQ_URL")
	if err != nil || val != "https://example.com/path" {
		t.Errorf("got %q err=%v", val, err)
	}
	os.Unsetenv("TEST_REQ_URL")
}

func TestRequireEnvURLWithDefault(t *testing.T) {
	os.Unsetenv("TEST_URL_DFT")
	if got := RequireEnvURLWithDefault("TEST_URL_DFT", "https://default.example"); got != "https://default.example" {
		t.Errorf("got %q", got)
	}

	os.Setenv("TEST_URL_DFT", "https://override.example")
	if got := RequireEnvURLWithDefault("TEST_URL_DFT", "https://default.example"); got != "https://override.example" {
		t.Errorf("got %q", got)
	}

	os.Setenv("TEST_URL_DFT", "not-a-url")
	if got := RequireEnvURLWithDefault("TEST_URL_DFT", "https://fallback.example"); got != "https://fallback.example" {
		t.Errorf("invalid URL should fall back, got %q", got)
	}
	os.Unsetenv("TEST_URL_DFT")
}

func TestRequireEnvSecret_changeMeRejected(t *testing.T) {
	os.Setenv("TEST_SECRET_CM", "please-change-me-now")
	defer os.Unsetenv("TEST_SECRET_CM")
	if _, err := RequireEnvSecret("TEST_SECRET_CM", 16); err == nil {
		t.Error("expected rejection when secret contains change-me")
	}
}

func TestServiceConfigValidator(t *testing.T) {
	os.Unsetenv("SVC_REQ_A")
	os.Unsetenv("SVC_REQ_B")

	v := NewServiceConfigValidator("test-service")
	if v.Require("SVC_REQ_A") != "" {
		t.Error("expected empty when missing")
	}
	if !v.HasErrors() {
		t.Error("expected errors after missing Require")
	}
	if len(v.Errors()) == 0 {
		t.Error("expected error strings")
	}
	if err := v.Validate(); err == nil {
		t.Error("expected Validate error")
	}

	v2 := NewServiceConfigValidator("svc2")
	os.Setenv("SVC_URL_OK", "https://ok.example")
	defer os.Unsetenv("SVC_URL_OK")
	if got := v2.RequireURL("SVC_URL_OK"); got != "https://ok.example" {
		t.Errorf("RequireURL: %q", got)
	}

	v3 := NewServiceConfigValidator("svc3")
	os.Setenv("SVC_GOOD_SECRET", "super-secret-value-long")
	defer os.Unsetenv("SVC_GOOD_SECRET")
	if got := v3.RequireSecret("SVC_GOOD_SECRET", 8); got == "" {
		t.Error("expected secret value")
	}

	v4 := NewServiceConfigValidator("svc4")
	v4.RequireAtLeastOne([]string{"MISSING_ONE", "MISSING_TWO"})
	if !v4.HasErrors() {
		t.Error("expected RequireAtLeastOne error")
	}

	v5 := NewServiceConfigValidator("svc5")
	if v5.RequireWithDefault("MISSING_WITH_DEF", "x") != "x" {
		t.Error("RequireWithDefault")
	}
	if v5.RequireIntWithDefault("MISSING_INT", 3) != 3 {
		t.Error("RequireIntWithDefault")
	}
	os.Setenv("SVC_INT", "9")
	defer os.Unsetenv("SVC_INT")
	if v5.RequireInt("SVC_INT") != 9 {
		t.Error("RequireInt")
	}
}

func TestValidateProduction_branches(t *testing.T) {
	oldEnv := os.Getenv("ENVIRONMENT")
	oldDbg := os.Getenv("DEBUG")
	oldDB := os.Getenv("DATABASE_URL")
	oldCRDB := os.Getenv("CRDB_DSN")
	defer func() {
		restore := func(k, v string) {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
		restore("ENVIRONMENT", oldEnv)
		restore("DEBUG", oldDbg)
		restore("DATABASE_URL", oldDB)
		restore("CRDB_DSN", oldCRDB)
	}()

	os.Setenv("ENVIRONMENT", "staging")
	os.Unsetenv("DEBUG")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("CRDB_DSN")
	if err := ValidateProduction(); err != nil {
		t.Errorf("non-production should pass: %v", err)
	}

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("DEBUG", "true")
	if err := ValidateProduction(); err == nil {
		t.Error("expected error when DEBUG=true in production")
	}

	os.Setenv("DEBUG", "false")
	os.Setenv("DATABASE_URL", "postgres://x?sslmode=disable")
	if err := ValidateProduction(); err == nil {
		t.Error("expected error for sslmode=disable in production")
	}
}
