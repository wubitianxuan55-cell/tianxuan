package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAllChecksSkipsWhenNoDeps(t *testing.T) {
	report := Run(context.Background(), Deps{})
	if len(report.Items) != 8 {
		t.Errorf("expected 8 items, got %d", len(report.Items))
	}
	// All items should be Skip when no deps
	for _, it := range report.Items {
		if it.Status != Skip && it.Name != "环境" && it.Name != "Go" {
			t.Errorf("%s: expected Skip, got %s", it.Name, it.Status)
		}
	}
}

func TestEnvironmentCheck(t *testing.T) {
	report := Run(context.Background(), Deps{CWD: "/tmp/test"})
	var env *Item
	for i := range report.Items {
		if report.Items[i].Name == "环境" {
			env = &report.Items[i]
			break
		}
	}
	if env == nil {
		t.Fatal("environment check not found")
	}
	if env.Status != Pass {
		t.Errorf("expected Pass, got %s: %s", env.Status, env.Detail)
	}
	if !strings.Contains(env.Detail, "shell=") {
		t.Errorf("detail should contain shell info: %s", env.Detail)
	}
}

func TestGoEnvCheck(t *testing.T) {
	report := Run(context.Background(), Deps{})
	var goItem *Item
	for i := range report.Items {
		if report.Items[i].Name == "Go" {
			goItem = &report.Items[i]
			break
		}
	}
	if goItem == nil {
		t.Fatal("Go check not found")
	}
	_, err := os.Stat("/usr/local/go/bin/go")
	if err == nil {
		// go is installed — should be Pass or Warn
		if goItem.Status == Skip {
			t.Errorf("go found on system but check skipped: %s", goItem.Detail)
		}
	}
}

func TestProjectCheck(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)

	report := Run(context.Background(), Deps{CWD: dir, GoModExists: true})
	var proj *Item
	for i := range report.Items {
		if report.Items[i].Name == "项目" {
			proj = &report.Items[i]
			break
		}
	}
	if proj == nil {
		t.Fatal("project check not found")
	}
	if proj.Status != Pass {
		t.Errorf("expected Pass, got %s: %s", proj.Status, proj.Detail)
	}
	if !strings.Contains(proj.Detail, "example.com/test") {
		t.Errorf("expected module name in detail: %s", proj.Detail)
	}
}

func TestConfigCheckFailsWithoutModel(t *testing.T) {
	// deps with nil Cfg should skip
	report := Run(context.Background(), Deps{Cfg: nil})
	for _, it := range report.Items {
		if it.Name == "配置" {
			if it.Status != Skip {
				t.Errorf("config check should skip when Cfg is nil, got %s", it.Status)
			}
		}
	}
}

func TestSummaryFormatting(t *testing.T) {
	r := &Report{
		Items:  []Item{
			{Name: "a", Status: Pass},
			{Name: "b", Status: Pass},
			{Name: "c", Status: Warn},
		},
		Passed: 2,
		Warned: 1,
	}
	s := r.Summary()
	if !strings.Contains(s, "2/3") || !strings.Contains(s, "1 警告") {
		t.Errorf("unexpected summary: %s", s)
	}
}
