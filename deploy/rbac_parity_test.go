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

package deploy

import (
	"os"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// SUPPORT2-033 §9: RBAC parity against the controller's actual operations.
// The ruleset below is the exact authority the binary requires. Any source
// change adding a new API verb must update deploy/manifests/rbac.yaml AND
// this test together.

type rbacRule struct {
	APIGroups []string `json:"apiGroups"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}

type rbacDoc struct {
	Kind  string     `json:"kind"`
	Rules []rbacRule `json:"rules"`
}

func parseRBACRules(t *testing.T, manifest string) []rbacRule {
	t.Helper()
	var rules []rbacRule
	for _, doc := range strings.Split(manifest, "\n---\n") {
		var d rbacDoc
		if err := yaml.Unmarshal([]byte(doc), &d); err != nil {
			t.Fatalf("yaml unmarshal: %v", err)
		}
		if d.Kind != "ClusterRole" {
			continue
		}
		rules = append(rules, d.Rules...)
	}
	return rules
}

func TestRBACParity(t *testing.T) {
	data, err := os.ReadFile("manifests/rbac.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rules := parseRBACRules(t, string(data))
	if len(rules) != 6 {
		t.Fatalf("expected exactly 6 ClusterRole rules, got %d", len(rules))
	}
	want := []struct {
		groups    string
		resources string
		verbs     string
	}{
		{"cleaner.zen-mesh.io", "zencleanerpolicies", "get list watch"},
		{"cleaner.zen-mesh.io", "zencleanerpolicies/status", "get update patch"},
		{"*", "*", "get list watch delete"},
		{"", "namespaces", "get list watch"},
		{"coordination.k8s.io", "leases", "get list watch create update patch"},
		{"", "events", "create patch"},
	}
	for i, w := range want {
		got := rules[i]
		if strings.Join(got.APIGroups, ",") != w.groups ||
			strings.Join(got.Resources, ",") != w.resources ||
			strings.Join(got.Verbs, " ") != w.verbs {
			t.Errorf("rule %d drifted:\n got  groups=%v resources=%v verbs=%v\n want groups=%q resources=%q verbs=%q",
				i, got.APIGroups, got.Resources, got.Verbs, w.groups, w.resources, w.verbs)
		}
	}
	for _, v := range rules[2].Verbs {
		switch v {
		case "create", "update", "patch", "deletecollection", "impersonate", "bind", "escalate":
			t.Errorf("cleanup rule carries forbidden verb %q", v)
		}
	}
	for _, r := range rules {
		for _, res := range r.Resources {
			if res == "secrets" {
				t.Error("a dedicated secrets rule must not exist")
			}
		}
	}
}
