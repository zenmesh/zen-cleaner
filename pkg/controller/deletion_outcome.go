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

package controller

// SUPPORT2-033 §4 (atomic UID deletion): truthful per-object deletion
// outcomes. A deletion effect is only ever counted/classified as performed
// when the object actually was; refusals and server-side precondition
// conflicts are non-deletions and must never degrade into fake successes.

import (
	"errors"
	"fmt"

	"github.com/zenmesh/zen-cleaner/pkg/safety"
)

// DeletionOutcome classifies what one delete attempt actually did to the
// object it targeted (OBSERVED effect, never the requested action).
type DeletionOutcome int

const (
	// OutcomeDeleted: the DELETE call was issued and accepted by the API
	// server for an object whose UID and resourceVersion matched the
	// preconditions (exactly one deletion of exactly that object).
	OutcomeDeleted DeletionOutcome = iota
	// OutcomeAlreadyAbsent: the object was already gone; nothing was deleted
	// (idempotent no-op success).
	OutcomeAlreadyAbsent
	// OutcomeRefused: a safety/policy/UID check refused BEFORE any DELETE was
	// issued; the object survives.
	OutcomeRefused
	// OutcomePreconditionConflict: the API server rejected the UID or
	// resourceVersion precondition (the object was replaced or updated
	// between the deletion evaluation and the DELETE call); the object
	// survives.
	OutcomePreconditionConflict
	// OutcomeFailed: the delete mechanism itself failed (API error, not a
	// refusal); the object's fate is unknown from this call.
	OutcomeFailed
)

// DeletionOutcomeRecord is the truthful outcome of one object's deletion
// attempt, with the bounded refusal reason when it was not deleted.
type DeletionOutcomeRecord struct {
	Outcome DeletionOutcome
	// Reason is a bounded pkg/safety refusal vocabulary value; empty when
	// the object was deleted or was already absent.
	Reason string
	// Detail carries the non-identity explanation for logs/receipts.
	Detail string
}

// Deleted reports whether the outcome represents an actual deletion effect.
func (r DeletionOutcomeRecord) Deleted() bool {
	return r.Outcome == OutcomeDeleted
}

// DeletionRefusedError reports a DELIBERATE non-deletion: the object was NOT
// deleted because a safety/policy/UID check refused or the API server
// rejected the deletion preconditions. It is never a failure of the delete
// mechanism and never counts as a deletion effect. Callers distinguish
// refusals from failures with errors.As; the Reason is a bounded
// pkg/safety vocabulary value.
type DeletionRefusedError struct {
	// Reason is the bounded refusal reason (pkg/safety).
	Reason string
	// Detail explains which check refused (no object dumps, no secrets).
	Detail string
	// Cause is the sentinel cause when one exists (e.g. ErrStaleUID).
	Cause error
}

func (e *DeletionRefusedError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("deletion refused (%s): %s", e.Reason, e.Detail)
	}
	return fmt.Sprintf("deletion refused (%s)", e.Reason)
}

func (e *DeletionRefusedError) Unwrap() error {
	return e.Cause
}

// refusalErr builds a *DeletionRefusedError from an outcome record.
func refusalErr(rec DeletionOutcomeRecord) *DeletionRefusedError {
	var cause error
	if rec.Reason == safety.ReasonStaleUID {
		cause = ErrStaleUID
	}
	return &DeletionRefusedError{Reason: rec.Reason, Detail: rec.Detail, Cause: cause}
}

// isDeletionRefused reports whether err is a deliberate non-deletion
// (refusal/precondition conflict) and returns it. Refusals are never
// deletion failures and never count as deletion effects.
func isDeletionRefused(err error) (*DeletionRefusedError, bool) {
	var refused *DeletionRefusedError
	if errors.As(err, &refused) {
		return refused, true
	}
	return nil, false
}

// ErrAlreadyAbsent is the typed signal that the targeted object was already
// gone before any DELETE was issued (or vanished between the precondition
// read and the DELETE call). It is a truthful NON-event: nothing was deleted
// by this path, so it must never be counted as a deletion, never fire a
// "deleted" event/metric, and never be retried. It travels the error-only
// deleter interface so absence cannot masquerade as a performed deletion.
var ErrAlreadyAbsent = errors.New("already absent: nothing was deleted")

// recordRefusedOutcome stores a truthful non-deletion outcome for one object
// when a per-object outcome collection is active (receipt path); it is a
// no-op otherwise.
func recordRefusedOutcome(outcomes map[string]DeletionOutcomeRecord, key string, refused *DeletionRefusedError) {
	if outcomes == nil {
		return
	}
	outcome := OutcomeRefused
	if refused.Reason == safety.ReasonPreconditionConflict {
		outcome = OutcomePreconditionConflict
	}
	outcomes[key] = DeletionOutcomeRecord{Outcome: outcome, Reason: refused.Reason, Detail: refused.Detail}
}
