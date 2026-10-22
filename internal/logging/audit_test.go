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

package logging

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestAuditConstants(t *testing.T) {
	actions := map[AuditAction]string{
		AuditActionCreate:       "create",
		AuditActionRead:         "read",
		AuditActionUpdate:       "update",
		AuditActionDelete:       "delete",
		AuditActionLogin:        "login",
		AuditActionLogout:       "logout",
		AuditActionAuthorize:    "authorize",
		AuditActionConfigChange: "config_change",
		AuditActionDataAccess:   "data_access",
	}
	for action, want := range actions {
		if string(action) != want {
			t.Errorf("audit action = %q, want %q", action, want)
		}
	}
	results := map[AuditResult]string{
		AuditResultSuccess: "success",
		AuditResultFailure: "failure",
		AuditResultDenied:  "denied",
	}
	for result, want := range results {
		if string(result) != want {
			t.Errorf("audit result = %q, want %q", result, want)
		}
	}
}

func TestAuditFieldHelpers(t *testing.T) {
	tests := []struct {
		name  string
		field zap.Field
		key   string
		value string
	}{
		{"AuditActionField", AuditActionField(AuditActionDelete), "audit_action", "delete"},
		{"AuditResultField", AuditResultField(AuditResultDenied), "audit_result", "denied"},
		{"AuditTypeField", AuditTypeField("user_action"), "audit_type", "user_action"},
		{"PermissionField", PermissionField("pods/list"), "permission", "pods/list"},
		{"ConfigKeyField", ConfigKeyField("max_retries"), "config_key", "max_retries"},
		{"DataTypeField", DataTypeField("pii"), "data_type", "pii"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireStringField(t, []zap.Field{tt.field}, tt.key, tt.value)
		})
	}
}

// auditTestCtx returns a context carrying a user id long enough to be masked.
func auditTestCtx() context.Context {
	return WithUserID(context.Background(), "12345678-90ab-41d4-a716-446655440000")
}

func TestAuditLoggerLogUserAction(t *testing.T) {
	logger, logs := newObservedLogger()
	al := NewAuditLogger(logger)
	al.LogUserAction(auditTestCtx(), AuditActionUpdate, "pod", "pod-1", AuditResultSuccess,
		zap.String("extra", "x"))

	fields := lastEntryFields(t, logs)
	requireStringField(t, fields, "audit_type", "user_action")
	requireStringField(t, fields, "audit_action", "update")
	requireStringField(t, fields, "audit_result", "success")
	requireStringField(t, fields, "resource_type", "pod")
	requireStringField(t, fields, "resource_id", "pod-1")
	requireStringField(t, fields, "extra", "x")
	// User id from context must be masked.
	requireStringField(t, fields, "user_id", "12345678...")
}

func TestAuditLoggerSkipsMissingUserContext(t *testing.T) {
	logger, logs := newObservedLogger()
	al := NewAuditLogger(logger)
	al.LogUserAction(context.Background(), AuditActionRead, "secret", "s-1", AuditResultFailure)

	fields := lastEntryFields(t, logs)
	requireNoField(t, fields, "user_id")
	requireStringField(t, fields, "audit_result", "failure")
}

func TestAuditLoggerLogLogin(t *testing.T) {
	logger, logs := newObservedLogger()
	al := NewAuditLogger(logger)
	al.LogLogin(auditTestCtx(), AuditResultFailure, "203.0.113.7", "agent/1.0")

	fields := lastEntryFields(t, logs)
	requireStringField(t, fields, "audit_type", "authentication")
	requireStringField(t, fields, "audit_action", "login")
	requireStringField(t, fields, "audit_result", "failure")
	// IP must be logged masked.
	requireStringField(t, fields, "remote_addr", "203.x.x.x")
	requireStringField(t, fields, "user_agent", "agent/1.0")
	requireStringField(t, fields, "user_id", "12345678...")
}

func TestAuditLoggerLogAuthorization(t *testing.T) {
	logger, logs := newObservedLogger()
	al := NewAuditLogger(logger)
	al.LogAuthorization(auditTestCtx(), "configmap", "cm-1", "get", AuditResultDenied)

	fields := lastEntryFields(t, logs)
	requireStringField(t, fields, "audit_type", "authorization")
	requireStringField(t, fields, "audit_action", "authorize")
	requireStringField(t, fields, "audit_result", "denied")
	requireStringField(t, fields, "permission", "get")
	requireStringField(t, fields, "resource_id", "cm-1")
}

func TestAuditLoggerLogConfigChange(t *testing.T) {
	t.Run("with values", func(t *testing.T) {
		logger, logs := newObservedLogger()
		al := NewAuditLogger(logger)
		al.LogConfigChange(context.Background(), "level", "info", "debug")

		fields := lastEntryFields(t, logs)
		requireStringField(t, fields, "audit_type", "config_change")
		requireStringField(t, fields, "audit_result", "success")
		requireStringField(t, fields, "config_key", "level")
		requireStringField(t, fields, "config_old_value", "info")
		requireStringField(t, fields, "config_new_value", "debug")
	})

	t.Run("nil values are omitted", func(t *testing.T) {
		logger, logs := newObservedLogger()
		al := NewAuditLogger(logger)
		al.LogConfigChange(context.Background(), "level", nil, nil)

		fields := lastEntryFields(t, logs)
		requireNoField(t, fields, "config_old_value")
		requireNoField(t, fields, "config_new_value")
	})
}

func TestAuditLoggerLogDataAccess(t *testing.T) {
	logger, logs := newObservedLogger()
	al := NewAuditLogger(logger)
	al.LogDataAccess(auditTestCtx(), "pii", "record-9")

	fields := lastEntryFields(t, logs)
	requireStringField(t, fields, "audit_type", "data_access")
	requireStringField(t, fields, "data_type", "pii")
	requireStringField(t, fields, "resource_id", "record-9")
	requireStringField(t, fields, "user_id", "12345678...")
}

func TestAuditLoggerLogResourceOperation(t *testing.T) {
	logger, logs := newObservedLogger()
	al := NewAuditLogger(logger)
	al.LogResourceOperation(auditTestCtx(), AuditActionDelete, "deployment", "dep-1", AuditResultSuccess)

	fields := lastEntryFields(t, logs)
	requireStringField(t, fields, "audit_type", "resource_operation")
	requireStringField(t, fields, "audit_action", "delete")
	requireStringField(t, fields, "audit_result", "success")
	requireStringField(t, fields, "resource_type", "deployment")
	requireStringField(t, fields, "resource_id", "dep-1")
}

// Compile-time: audit logger must emit at info level by default.
func TestAuditLoggerLogsAtInfoLevel(t *testing.T) {
	logger, logs := newObservedLogger()
	al := NewAuditLogger(logger)
	al.LogUserAction(context.Background(), AuditActionCreate, "pod", "p", AuditResultSuccess)
	entries := logs.TakeAll()
	if len(entries) != 1 || entries[0].Level != zapcore.InfoLevel {
		t.Fatalf("expected one info entry, got %+v", entries)
	}
}
