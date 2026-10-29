package repo

// 2026-10-29: the AI discovery index (llms.txt) and docs/ai/status.json
// drifted to "0.0.1-alpha" while the release moved to 0.1.x (owner
// launch-validation follow-up). The chart's appVersion is the ONE
// canonical version source (HERMES-155 analog): these checks fail the
// suite when the AI-facing version strings drift from it.
import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func rootDir(t *testing.T) string {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLLMsIndexVersionMatchesChart(t *testing.T) {
	chart, err := os.ReadFile(filepath.Join(rootDir(t), "deploy", "helm", "zen-cleaner", "Chart.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`appVersion:\s*"([^"]+)"`).FindStringSubmatch(string(chart))
	if m == nil {
		t.Fatal("Chart.yaml appVersion not found")
	}
	appVersion := m[1]

	llms, err := os.ReadFile(filepath.Join(rootDir(t), "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(llms), "**Version:** "+appVersion) {
		t.Errorf("llms.txt version drifted from Chart.yaml appVersion %q — update llms.txt on release", appVersion)
	}
}

func TestAIStatusVersionMatchesChart(t *testing.T) {
	chart, err := os.ReadFile(filepath.Join(rootDir(t), "deploy", "helm", "zen-cleaner", "Chart.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`appVersion:\s*"([^"]+)"`).FindStringSubmatch(string(chart))
	if m == nil {
		t.Fatal("Chart.yaml appVersion not found")
	}
	appVersion := m[1]

	status, err := os.ReadFile(filepath.Join(rootDir(t), "docs", "ai", "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(status), `"version": "`+appVersion+`"`) {
		t.Errorf("docs/ai/status.json version drifted from Chart.yaml appVersion %q — update on release", appVersion)
	}
}
