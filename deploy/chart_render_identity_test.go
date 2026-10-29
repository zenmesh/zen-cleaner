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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// THE CHART RENDER-IDENTITY LAW (found live, MON soft-launch 2026-10-15):
// the cluster-wide delete branch renders EXACTLY ONCE regardless of
// rbac.deleteNamespaces (ClusterRoles are not per-namespace; the earlier
// template emitted duplicate ClusterRole/ClusterRoleBinding identities —
// invalid for kubectl apply and ambiguous for admission), and the namespaced
// branch renders exactly one Role+RoleBinding pair per UNIQUE intended
// namespace (release namespace deduped). This test renders the chart with
// helm across the combination matrix and asserts strict manifest identity.
func TestChartRenderIdentity_NoDuplicateIdentities(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed; chart render-identity test requires the helm CLI (skip, not pass)")
	}
	chartDir := filepath.Join("helm", "zen-cleaner")
	if _, err := os.Stat(chartDir); err != nil {
		t.Fatalf("chart directory missing: %v", err)
	}

	type objID struct{ kind, name, ns string }
	render := func(extra ...string) []objID {
		// Pin the namespace: helm template otherwise inherits the kubeconfig
		// CONTEXT namespace, making this test environment-dependent (observed
		// 2026-10-29: a shared-cluster context namespace silently flipped
		// every rendered namespace and failed the identity law).
		args := append([]string{"template", "identity-check", chartDir, "--namespace", "default"}, extra...)
		cmd := exec.Command(helm, args...)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("helm template %v: %v", extra, err)
		}
		var ids []objID
		for _, doc := range splitYAMLDocs(out) {
			var m struct {
				Kind     string `json:"kind"`
				Metadata struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"metadata"`
			}
			if err := yaml.Unmarshal(doc, &m); err != nil || m.Kind == "" {
				continue
			}
			ids = append(ids, objID{m.Kind, m.Metadata.Name, m.Metadata.Namespace})
		}
		return ids
	}
	deleteObjects := func(ids []objID) []string {
		var out []string
		for _, id := range ids {
			if strings.Contains(id.name, "delete") {
				ns := id.ns
				if ns == "" {
					ns = "cluster"
				}
				out = append(out, id.kind+":"+ns)
			}
		}
		sort.Strings(out)
		return out
	}
	assertNoDupes := func(label string, ids []objID) {
		seen := map[objID]int{}
		for _, id := range ids {
			seen[id]++
			if seen[id] > 1 {
				t.Errorf("%s: duplicate manifest identity %v", label, id)
			}
		}
	}
	assertDeleteSet := func(label string, ids []objID, want map[string]bool) {
		got := map[string]bool{}
		for _, o := range deleteObjects(ids) {
			got[o] = true
		}
		if len(got) != len(want) {
			t.Errorf("%s: delete objects %v != expected %v", label, got, want)
		}
		for w := range want {
			if !got[w] {
				t.Errorf("%s: missing expected delete object %s", label, w)
			}
		}
	}

	roleObjs := func(namespaces ...string) map[string]bool {
		out := map[string]bool{}
		for _, ns := range namespaces {
			out["Role:"+ns] = true
			out["RoleBinding:"+ns] = true
		}
		return out
	}
	// THE NULL-GROUP LAW, two halves (found live on the public install):
	// (a) a targetKinds MAP with an absent/empty group field renders the
	// CORE API group (RBAC "" convention) — the explicit two-index --set
	// spelling is the supported extension form;
	// (b) the naive reader spelling `--set rbac.targetKinds[1].resource=x`
	// against the one-entry default list makes Helm emit a NULL list
	// element, which previously rendered silently as an "?invalid-target"
	// rule in a live ClusterRole and Role while dropping the real jobs
	// allowlist. Corrupt RBAC must never render: the template now fails the
	// install loudly instead.
	t.Run("partial --set index fails loudly instead of shipping corrupt RBAC", func(t *testing.T) {
		args := append([]string{"template", "identity-check", chartDir},
			"--set", "rbac.targetKinds[1].resource=configmaps",
			"--set", "rbac.deleteNamespaces[0]=a")
		out, err := exec.Command(helm, args...).CombinedOutput()
		if err == nil {
			t.Fatalf("partial-index --set must fail the render; it produced manifests instead. Output:\n%s", out)
		}
		if !strings.Contains(string(out), "rbac.targetKinds entries must be maps") {
			t.Fatalf("expected the explicit targetKinds guidance in the failure, got:\n%s", out)
		}
	})

	t.Run("explicit two-index --set renders jobs and core-group configmaps", func(t *testing.T) {
		extension := []string{
			"--set", "rbac.targetKinds[0].group=batch", "--set", "rbac.targetKinds[0].resource=jobs",
			"--set", "rbac.targetKinds[1].group=", "--set", "rbac.targetKinds[1].resource=configmaps",
			"--set", "rbac.deleteNamespaces[0]=a"}
		args := append([]string{"template", "identity-check", chartDir}, extension...)
		out, err := exec.Command(helm, args...).Output()
		if err != nil {
			t.Fatalf("explicit two-index --set must render: %v", err)
		}
		rendered := string(out)
		for _, want := range []string{
			`apiGroups: ["batch"]`, // the jobs allowlist survives the extension
			`resources: ["jobs"`,
			`resources: ["configmaps"]`, // absent/empty group == core API group
		} {
			if !strings.Contains(rendered, want) {
				t.Fatalf("rendered RBAC missing %s; output:\n%s", want, rendered)
			}
		}
		if strings.Contains(rendered, "?invalid-target") {
			t.Fatal("rendered RBAC contains the ?invalid-target sentinel")
		}
		assertNoDupes("explicit-two-index", render(extension...))
	})

	cases := []struct {
		label string
		extra []string
		want  map[string]bool
	}{
		{"default", nil, roleObjs("default")},
		{"namespaced + two deleteNamespaces",
			[]string{"--set", "rbac.deleteNamespaces[0]=a", "--set", "rbac.deleteNamespaces[1]=b"},
			roleObjs("default", "a", "b")},
		{"namespaced + duplicate input namespaces (deduped)",
			[]string{"--set", "rbac.deleteNamespaces[0]=a", "--set", "rbac.deleteNamespaces[1]=a"},
			roleObjs("default", "a")},
		{"cluster scope + deleteNamespaces (rendered once, ignored)",
			[]string{"--set", "rbac.namespaceScope=false", "--set", "rbac.deleteNamespaces[0]=fixture-a"},
			map[string]bool{"ClusterRole:cluster": true, "ClusterRoleBinding:cluster": true}},
		{"cluster scope plain",
			[]string{"--set", "rbac.namespaceScope=false"},
			map[string]bool{"ClusterRole:cluster": true, "ClusterRoleBinding:cluster": true}},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			ids := render(tc.extra...)
			assertNoDupes(tc.label, ids)
			assertDeleteSet(tc.label, ids, tc.want)
		})
	}
}

func splitYAMLDocs(b []byte) [][]byte {
	var docs [][]byte
	start := -1
	for i := 0; i+3 < len(b); i++ {
		if b[i] == '-' && b[i+1] == '-' && b[i+2] == '-' && (i == 0 || b[i-1] == '\n') {
			if start >= 0 {
				docs = append(docs, b[start:i])
			}
			start = i + 3
		}
	}
	if start >= 0 && start < len(b) {
		docs = append(docs, b[start:])
	}
	return docs
}
