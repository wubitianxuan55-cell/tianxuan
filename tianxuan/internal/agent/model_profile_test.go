package agent

import "testing"

func TestLookupModelProfileFound(t *testing.T) {
	profiles := DefaultModelProfiles()
	p := LookupModelProfile("deepseek-v4-flash", profiles)
	if p == nil {
		t.Fatal("should find flash profile")
	}
	if p.ContextWindow != 128_000 {
		t.Errorf("flash window = %d, want 128000", p.ContextWindow)
	}
}

func TestLookupModelProfileNotFound(t *testing.T) {
	profiles := DefaultModelProfiles()
	p := LookupModelProfile("unknown-model", profiles)
	if p != nil {
		t.Errorf("should return nil for unknown model, got %+v", p)
	}
}

func TestApplyModelProfile(t *testing.T) {
	comp := CompactionConfig{Window: 100_000, Ratio: 0.8}
	profile := &ModelProfile{ContextWindow: 1_000_000, SoftRatio: 0.7}
	ApplyModelProfile(&comp, profile)

	if comp.Window != 1_000_000 {
		t.Errorf("Window = %d, want 1000000", comp.Window)
	}
	if comp.Ratio != 0.7 {
		t.Errorf("Ratio = %f, want 0.7", comp.Ratio)
	}
}

func TestApplyModelProfileNil(t *testing.T) {
	comp := CompactionConfig{Window: 100_000, Ratio: 0.8}
	ApplyModelProfile(&comp, nil)

	if comp.Window != 100_000 {
		t.Errorf("nil profile should not change Window, got %d", comp.Window)
	}
	if comp.Ratio != 0.8 {
		t.Errorf("nil profile should not change Ratio, got %f", comp.Ratio)
	}
}
