package agent

import (
	"strings"
	"testing"
)

// V10.141 契约：验证按变更范围分级——简单修改/前端修改不强制跑后端全量
// 测试套件，避免"每轮完成都 go test ./..." 的机械开销。

func TestVerifyGateNudge_ScopedVerification(t *testing.T) {
	for _, kw := range []string{"matches the change", "affected package tests", "frontend", "no backend tests needed", "docs/config", "full suite"} {
		if !strings.Contains(stopGateOrchestrateVerifyNudge, kw) {
			t.Errorf("verify-gate nudge missing scoped-verification keyword %q", kw)
		}
	}
}

func TestSoloSystemPrompt_ScopedVerification(t *testing.T) {
	p := SoloSystemPrompt
	for _, kw := range []string{"matches the change", "affected package tests", "frontend", "docs/config", "full suite"} {
		if !strings.Contains(p, kw) {
			t.Errorf("SoloSystemPrompt missing scoped-verification keyword %q", kw)
		}
	}
}

func TestHephaestusSystemPrompt_ScopedVerification(t *testing.T) {
	p := HephaestusSystemPrompt
	for _, kw := range []string{"matches the change", "affected package tests", "frontend", "full suite"} {
		if !strings.Contains(p, kw) {
			t.Errorf("HephaestusSystemPrompt missing scoped-verification keyword %q", kw)
		}
	}
}
