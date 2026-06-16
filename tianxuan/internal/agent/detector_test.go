package agent

import "testing"

func TestDetectorDependencyBlindspot(t *testing.T) {
	d := NewDetector()
	outcomes := []toolOutcome{
		{errMsg: "file not found: internal/auth/middleware.go"},
		{errMsg: "file not found: internal/auth/middleware.go"},
	}
	report := d.Analyze("read_file|read_file", outcomes)
	if report == nil {
		t.Fatal("expected a report for file-not-found errors")
	}
	if report.Kind != KindDependencyBlindspot {
		t.Errorf("Kind = %q, want %q", report.Kind, KindDependencyBlindspot)
	}
	if report.Hint == "" {
		t.Error("Hint should not be empty")
	}
}

func TestDetectorToolMisuse(t *testing.T) {
	d := NewDetector()
	outcomes := []toolOutcome{
		{errMsg: "bash: command not found: nosuchcmd"},
		{errMsg: "bash: command not found: nosuchcmd"},
		{errMsg: "bash: command not found: nosuchcmd"},
	}
	report := d.Analyze("bash|bash|bash", outcomes)
	if report == nil {
		t.Fatal("expected a report for repeated tool failure")
	}
	if report.Kind != KindToolMisuse {
		t.Errorf("Kind = %q, want %q", report.Kind, KindToolMisuse)
	}
}

func TestDetectorNoPattern(t *testing.T) {
	d := NewDetector()
	outcomes := []toolOutcome{
		{errMsg: "something went wrong"},
	}
	report := d.Analyze("tool1", outcomes)
	if report != nil {
		t.Errorf("expected nil report, got %+v", report)
	}
}

func TestDetectorEmptyInput(t *testing.T) {
	d := NewDetector()
	if report := d.Analyze("", nil); report != nil {
		t.Error("expected nil for empty input")
	}
	if report := d.Analyze("sig", []toolOutcome{}); report != nil {
		t.Error("expected nil for empty outcomes")
	}
}
