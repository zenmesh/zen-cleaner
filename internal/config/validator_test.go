package config

import (
	"os"
	"testing"
)

func TestValidator_RequireString(t *testing.T) {
	v := NewValidator()

	// Test missing env var
	t.Setenv("TEST_REQUIRED", "")
	val := v.RequireString("TEST_REQUIRED")
	if val != "" {
		t.Errorf("Expected empty string, got %s", val)
	}
	if !v.HasErrors() {
		t.Error("Expected validation error")
	}

	// Test present env var
	t.Setenv("TEST_REQUIRED", "test-value")
	v2 := NewValidator()
	val = v2.RequireString("TEST_REQUIRED")
	if val != "test-value" {
		t.Errorf("Expected 'test-value', got %s", val)
	}
	if v2.HasErrors() {
		t.Error("Expected no validation errors")
	}
	t.Setenv("TEST_REQUIRED", "")
}

func TestValidator_OptionalString(t *testing.T) {
	v := NewValidator()

	// Test missing env var with default
	t.Setenv("TEST_OPTIONAL", "")
	val := v.OptionalString("TEST_OPTIONAL", "default-value")
	if val != "default-value" {
		t.Errorf("Expected 'default-value', got %s", val)
	}

	// Test present env var
	t.Setenv("TEST_OPTIONAL", "actual-value")
	val = v.OptionalString("TEST_OPTIONAL", "default-value")
	if val != "actual-value" {
		t.Errorf("Expected 'actual-value', got %s", val)
	}
	t.Setenv("TEST_OPTIONAL", "")
}

func TestValidator_RequireInt(t *testing.T) {
	v := NewValidator()

	// Test missing env var
	t.Setenv("TEST_INT", "")
	val := v.RequireInt("TEST_INT")
	if val != 0 {
		t.Errorf("Expected 0, got %d", val)
	}
	if !v.HasErrors() {
		t.Error("Expected validation error")
	}

	// Test invalid int
	t.Setenv("TEST_INT", "not-a-number")
	v2 := NewValidator()
	val = v2.RequireInt("TEST_INT")
	if val != 0 {
		t.Errorf("Expected 0, got %d", val)
	}
	if !v2.HasErrors() {
		t.Error("Expected validation error")
	}

	// Test valid int
	t.Setenv("TEST_INT", "42")
	v3 := NewValidator()
	val = v3.RequireInt("TEST_INT")
	if val != 42 {
		t.Errorf("Expected 42, got %d", val)
	}
	if v3.HasErrors() {
		t.Error("Expected no validation errors")
	}
	t.Setenv("TEST_INT", "")
}

func TestValidator_RequireURL(t *testing.T) {
	v := NewValidator()

	// Test missing env var
	t.Setenv("TEST_URL", "")
	val := v.RequireURL("TEST_URL")
	if val != "" {
		t.Errorf("Expected empty string, got %s", val)
	}
	if !v.HasErrors() {
		t.Error("Expected validation error")
	}

	// Test invalid URL
	t.Setenv("TEST_URL", "not-a-url")
	v2 := NewValidator()
	val = v2.RequireURL("TEST_URL")
	if val != "" {
		t.Errorf("Expected empty string, got %s", val)
	}
	if !v2.HasErrors() {
		t.Error("Expected validation error")
	}

	// Test valid URL
	t.Setenv("TEST_URL", "https://example.com")
	v3 := NewValidator()
	val = v3.RequireURL("TEST_URL")
	if val != "https://example.com" {
		t.Errorf("Expected 'https://example.com', got %s", val)
	}
	if v3.HasErrors() {
		t.Error("Expected no validation errors")
	}
	t.Setenv("TEST_URL", "")
}

func TestValidator_RequireCSV(t *testing.T) {
	v := NewValidator()

	// Test missing env var
	t.Setenv("TEST_CSV", "")
	val := v.RequireCSV("TEST_CSV")
	if val != nil {
		t.Errorf("Expected nil, got %v", val)
	}
	if !v.HasErrors() {
		t.Error("Expected validation error")
	}

	// Test valid CSV
	t.Setenv("TEST_CSV", "a,b,c")
	v2 := NewValidator()
	val = v2.RequireCSV("TEST_CSV")
	if len(val) != 3 {
		t.Errorf("Expected 3 items, got %d", len(val))
	}
	if v2.HasErrors() {
		t.Error("Expected no validation errors")
	}
	t.Setenv("TEST_CSV", "")
}

func TestValidator_Validate(t *testing.T) {
	v := NewValidator()

	// No errors
	err := v.Validate()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// With errors
	t.Setenv("TEST_REQUIRED", "")
	v.RequireString("TEST_REQUIRED")
	err = v.Validate()
	if err == nil {
		t.Error("Expected validation error")
	}
	t.Setenv("TEST_REQUIRED", "")
}

func TestValidator_OptionalURL(t *testing.T) {
	v := NewValidator()
	t.Setenv("OPT_URL", "")
	if got := v.OptionalURL("OPT_URL", "https://default.example"); got != "https://default.example" {
		t.Errorf("got %q", got)
	}

	t.Setenv("OPT_URL", "not-url")
	if got := v.OptionalURL("OPT_URL", "https://fallback.example"); got != "https://fallback.example" {
		t.Errorf("invalid URL should use default, got %q", got)
	}
	if !v.HasErrors() {
		t.Error("expected validation error for invalid URL")
	}

	v2 := NewValidator()
	t.Setenv("OPT_URL2", "https://ok.example")
	if got := v2.OptionalURL("OPT_URL2", "https://default.example"); got != "https://ok.example" {
		t.Errorf("got %q", got)
	}
	t.Setenv("OPT_URL", "")
	t.Setenv("OPT_URL2", "")
}

func TestValidator_OptionalInt(t *testing.T) {
	v := NewValidator()
	t.Setenv("OPT_INT", "")
	if v.OptionalInt("OPT_INT", 7) != 7 {
		t.Error("default int")
	}

	t.Setenv("OPT_INT", "nope")
	v2 := NewValidator()
	if v2.OptionalInt("OPT_INT", 42) != 42 {
		t.Error("invalid int uses default")
	}
	if !v2.HasErrors() {
		t.Error("expected error for bad int")
	}

	t.Setenv("OPT_INT", "99")
	v3 := NewValidator()
	if v3.OptionalInt("OPT_INT", 1) != 99 {
		t.Error("parsed int")
	}
	t.Setenv("OPT_INT", "")
}

func TestValidator_RequireBool_and_OptionalBool(t *testing.T) {
	v := NewValidator()
	t.Setenv("REQ_BOOL", "")
	if v.RequireBool("REQ_BOOL") {
		t.Error("missing bool should be false")
	}
	if !v.HasErrors() {
		t.Error("expected error")
	}

	t.Setenv("REQ_BOOL", "not-bool")
	v2 := NewValidator()
	v2.RequireBool("REQ_BOOL")
	if !v2.HasErrors() {
		t.Error("invalid bool")
	}

	t.Setenv("REQ_BOOL", "true")
	v3 := NewValidator()
	if !v3.RequireBool("REQ_BOOL") {
		t.Error("true")
	}

	t.Setenv("OPT_BOOL", "")
	v4 := NewValidator()
	if !v4.OptionalBool("OPT_BOOL", true) {
		t.Error("optional default true")
	}

	t.Setenv("OPT_BOOL", "false")
	v5 := NewValidator()
	if v5.OptionalBool("OPT_BOOL", true) {
		t.Error("optional false")
	}
	t.Setenv("REQ_BOOL", "")
	t.Setenv("OPT_BOOL", "")
}

func TestValidator_Duration_helpers(t *testing.T) {
	v := NewValidator()
	t.Setenv("REQ_DUR", "")
	if v.RequireDuration("REQ_DUR") != "" {
		t.Error("empty when missing")
	}
	if !v.HasErrors() {
		t.Error("error when missing")
	}

	t.Setenv("REQ_DUR", "bad")
	v2 := NewValidator()
	v2.RequireDuration("REQ_DUR")
	if !v2.HasErrors() {
		t.Error("bad duration suffix")
	}

	t.Setenv("REQ_DUR", "30s")
	v3 := NewValidator()
	if v3.RequireDuration("REQ_DUR") != "30s" {
		t.Errorf("got %q", v3.RequireDuration("REQ_DUR"))
	}

	t.Setenv("OPT_DUR", "")
	v4 := NewValidator()
	if v4.OptionalDuration("OPT_DUR", "5m") != "5m" {
		t.Error("optional default")
	}

	t.Setenv("OPT_DUR", "bad")
	v5 := NewValidator()
	if v5.OptionalDuration("OPT_DUR", "1h") != "1h" {
		t.Error("invalid optional uses default")
	}
	t.Setenv("REQ_DUR", "")
	t.Setenv("OPT_DUR", "")
}

func TestValidator_RequireOneOf(t *testing.T) {
	v := NewValidator()
	t.Setenv("ONEOF", "")
	if v.RequireOneOf("ONEOF", []string{"a", "b"}) != "" {
		t.Error("empty when unset")
	}

	t.Setenv("ONEOF", "c")
	v2 := NewValidator()
	v2.RequireOneOf("ONEOF", []string{"a", "b"})
	if !v2.HasErrors() {
		t.Error("not in allowed")
	}

	t.Setenv("ONEOF", "b")
	v3 := NewValidator()
	if v3.RequireOneOf("ONEOF", []string{"a", "b"}) != "b" {
		t.Error("match")
	}
	t.Setenv("ONEOF", "")
}

func TestValidator_OptionalCSV(t *testing.T) {
	v := NewValidator()
	t.Setenv("OCSV", "")
	def := []string{"x"}
	gotDef := v.OptionalCSV("OCSV", def)
	if len(gotDef) != 1 || gotDef[0] != "x" {
		t.Errorf("default CSV: %#v", gotDef)
	}

	t.Setenv("OCSV", "a, b , ")
	v2 := NewValidator()
	got := v2.OptionalCSV("OCSV", def)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %#v", got)
	}
	t.Setenv("OCSV", "")
}

func TestValidator_ForbidInProduction(t *testing.T) {
	old := os.Getenv("ENVIRONMENT")
	defer func() {
		if old == "" {
			t.Setenv("ENVIRONMENT", "")
		} else {
			t.Setenv("ENVIRONMENT", old)
		}
		t.Setenv("DEVONLY", "")
	}()

	t.Setenv("DEVONLY", "")
	t.Setenv("ENVIRONMENT", "development")
	v := NewValidator()
	v.ForbidInProduction("DEVONLY")
	if v.HasErrors() {
		t.Error("no error outside production")
	}

	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("DEVONLY", "1")
	v2 := NewValidator()
	v2.ForbidInProduction("DEVONLY")
	if !v2.HasErrors() {
		t.Error("forbidden in prod")
	}
}

func TestValidator_Errors_slice(t *testing.T) {
	v := NewValidator()
	t.Setenv("MULTI_MISSING", "")
	v.RequireString("MULTI_MISSING")
	v.RequireInt("MULTI_MISSING")
	errs := v.Errors()
	if len(errs) < 2 {
		t.Errorf("expected multiple ValidationErrors, got %d", len(errs))
	}
}
