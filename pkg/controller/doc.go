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

// Package controller implements the ZenCleanerPolicy reconciler, policy
// evaluation, metrics, status updates, and Kubernetes-facing helpers (informers,
// listing, deletion, health).
//
// # Evaluation paths
//
// evaluatePolicy tries, in order:
//
//  1. Primary: PolicyEvaluationService (see evaluate_policy_refactored.go) lists
//     targets via a ResourceLister backed by the policy's resource informer,
//     using injected selector/condition/rate-limit/delete adapters (adapters.go,
//     infrastructure.go). A single service instance is cached on the reconciler
//     after the first successful build (getOrCreateEvaluationService); call sites
//     should assume that cache is process-wide for that reconciler instance.
//
//  2. Fallback: if the service cannot be constructed, the same reconcile uses
//     evaluate_policy_shared.go helpers that read the informer store directly
//     (evaluatePolicyResourcesShared, deleteResourcesInBatchesShared,
//     updatePolicyStatusShared).
//
// Batch sizing for deletes is centralized in batch_config.go (resolveBatchSize).
//
// # Related packages
//
// Webhook admission lives in pkg/webhook. CRD types are pkg/api/v1alpha1.
package controller
