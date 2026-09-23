package manifests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// SUPPORT2-043 from-zero regression: the webhook Service selected
// app.kubernetes.io/name while the Deployment pods carried only app=, so the
// canonical install shipped a Service with no endpoints — admission webhooks
// unreachable and every policy application failing closed. Guards that every
// Service selector key/value in this directory is present in the Deployment
// pod template labels.
func TestServiceSelectorsMatchPodLabels(t *testing.T) {
	dep, err := os.ReadFile("deployment.yaml")
	if err != nil {
		t.Fatal(err)
	}
	podLabels := map[string]string{}
	// crude but pinned: every `k: v` pair under the pod template labels block
	inTemplate := false
	for _, ln := range strings.Split(string(dep), "\n") {
		if strings.Contains(ln, "template:") {
			inTemplate = true
			continue
		}
		if inTemplate {
			m := regexp.MustCompile(`^(\s+)([\w.\-/]+):\s*(\S+)\s*$`).FindStringSubmatch(ln)
			if m != nil && strings.HasPrefix(m[2], "app") {
				podLabels[m[2]] = m[3]
			}
		}
	}
	if len(podLabels) == 0 {
		t.Fatal("no pod template labels parsed from deployment.yaml")
	}

	files, _ := filepath.Glob("*.yaml")
	for _, f := range files {
		if f == "deployment.yaml" {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := string(data)
		if !strings.Contains(s, "kind: Service") && !strings.Contains(s, "kind: PodDisruptionBudget") {
			continue
		}
		sel := regexp.MustCompile(`selector:[\s\S]*?matchLabels:\n((?:\s+[\w.\-/]+:\s*\S+\n)+)`)
		if m := sel.FindStringSubmatch(s); m != nil {
			for _, pair := range strings.Split(strings.TrimSpace(m[1]), "\n") {
				kv := regexp.MustCompile(`^\s*([\w.\-/]+):\s*(\S+)`).FindStringSubmatch(pair)
				if kv == nil {
					continue
				}
				key, val := kv[1], kv[2]
				if podLabels[key] != strings.Trim(val, `"`) {
					t.Errorf("%s: selector {%s: %s} does not match any pod template label (webhook-dead defect class)", f, key, val)
				}
			}
		}
	}
}
