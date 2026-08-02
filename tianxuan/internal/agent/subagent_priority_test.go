package agent

import (
	"strings"
	"testing"
)

// V10.139 契约：双模型两侧提示词同样强化"调查默认子代理"——Hephaestus
// 执行阶段的编辑锚点调查与 Hermes 规划阶段的并行调查都不得在主上下文
// 铺开批量 read_file/grep。
func TestHephaestusSystemPrompt_SubagentPriority(t *testing.T) {
	p := HephaestusSystemPrompt
	for _, kw := range []string{"explore", "isolated", "don't batch"} {
		if !strings.Contains(p, kw) {
			t.Errorf("HephaestusSystemPrompt missing subagent-priority keyword %q", kw)
		}
	}
}

func TestHermesPrompt_SubagentPriority(t *testing.T) {
	p := HermesPrompt
	for _, kw := range []string{"parallel", "explore", "research"} {
		if !strings.Contains(p, kw) {
			t.Errorf("HermesPrompt missing subagent-priority keyword %q", kw)
		}
	}
}
