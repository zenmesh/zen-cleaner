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

import (
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

// resolveBatchSize returns the batch size for deletions: policy behavior overrides
// controller config, which overrides DefaultBatchSize.
func resolveBatchSize(policy *v1alpha1.ZenCleanerPolicy, ctrlCfg *config.ControllerConfig) int {
	batchSize := DefaultBatchSize
	if ctrlCfg != nil {
		batchSize = ctrlCfg.BatchSize
	}
	if policy.Spec.Behavior.BatchSize > 0 {
		batchSize = policy.Spec.Behavior.BatchSize
	}
	return batchSize
}
