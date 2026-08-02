package main

import "testing"

func TestParseWorktreePorcelain(t *testing.T) {
	out := `worktree /repo/main
HEAD a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0
branch refs/heads/main
main

worktree /repo/feature-x
HEAD 1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0
branch refs/heads/feature-x

worktree /repo/experiment
HEAD deadbeefdeadbeefdeadbeefdeadbeefdeadbeef
detached
`
	views := parseWorktreePorcelain(out)
	if len(views) != 3 {
		t.Fatalf("want 3 worktrees, got %d", len(views))
	}
	main := views[0]
	if main.Path != "/repo/main" || main.Branch != "main" || main.Detached || !main.Current {
		t.Fatalf("unexpected main: %+v", main)
	}
	feature := views[1]
	if feature.Path != "/repo/feature-x" || feature.Branch != "feature-x" || feature.Detached || feature.Current {
		t.Fatalf("unexpected feature: %+v", feature)
	}
	experiment := views[2]
	if experiment.Path != "/repo/experiment" || experiment.Branch != "" || !experiment.Detached || !experiment.Current {
		t.Fatalf("unexpected experiment: %+v", experiment)
	}
}

func TestParseWorktreePorcelainEmptyAndGarbage(t *testing.T) {
	if views := parseWorktreePorcelain(""); len(views) != 0 {
		t.Fatalf("empty input should yield no worktrees, got %+v", views)
	}
	out := "worktree /only-path\n\nnot a worktree block\n\nworktree /second\nHEAD 123\nbranch refs/heads/x\n"
	views := parseWorktreePorcelain(out)
	if len(views) != 2 {
		t.Fatalf("want 2 valid blocks, got %d (%+v)", len(views), views)
	}
	if views[0].Path != "/only-path" || views[0].Branch != "" || views[0].Current {
		t.Fatalf("unexpected first: %+v", views[0])
	}
	if views[1].Path != "/second" || views[1].Branch != "x" {
		t.Fatalf("unexpected second: %+v", views[1])
	}
}
