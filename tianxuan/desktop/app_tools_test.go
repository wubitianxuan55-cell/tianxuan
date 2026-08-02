package main

import (
	"sort"
	"testing"
)

func TestToolsReturnsAllBuiltins(t *testing.T) {
	var a App
	tools := a.Tools()
	if len(tools) == 0 {
		t.Fatal("expected at least one built-in tool")
	}
	seen := map[string]bool{}
	names := make([]string, 0, len(tools))
	for _, tv := range tools {
		if tv.Name == "" {
			t.Error("tool with empty name")
		}
		if seen[tv.Name] {
			t.Errorf("duplicate tool %q", tv.Name)
		}
		seen[tv.Name] = true
		names = append(names, tv.Name)
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("tools not sorted by name: %v", names)
	}
}

func TestToolsDescriptionsAndReadOnly(t *testing.T) {
	var a App
	byName := map[string]BuiltinToolView{}
	for _, tv := range a.Tools() {
		byName[tv.Name] = tv
	}
	rf, ok := byName["read_file"]
	if !ok {
		t.Fatal("read_file missing from built-in tools")
	}
	if rf.Description == "" || rf.FullDescription == "" {
		t.Errorf("read_file descriptions should not be empty: %+v", rf)
	}
	if !rf.ReadOnly {
		t.Error("read_file should be read-only")
	}
	bash, ok := byName["bash"]
	if !ok {
		t.Fatal("bash missing from built-in tools")
	}
	if bash.ReadOnly {
		t.Error("bash must not be read-only")
	}
	edit, ok := byName["edit_file"]
	if !ok {
		t.Fatal("edit_file missing from built-in tools")
	}
	if edit.Description == "" {
		t.Error("edit_file display description should not be empty")
	}
}
