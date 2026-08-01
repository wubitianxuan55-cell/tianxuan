package session

import "testing"

func TestStripTransientBlocksAutoSkill(t *testing.T) {
	in := "<auto-skill>\n[自动加载技能] tdd\nbody\n</auto-skill>\n\n用户原始输入"
	if got := StripTransientBlocks(in); got != "用户原始输入" {
		t.Errorf("auto-skill block should be stripped, got %q", got)
	}
	in2 := "<response-language>zh</response-language>\n<auto-skill>body</auto-skill>\n任务内容"
	if got := StripTransientBlocks(in2); got != "任务内容" {
		t.Errorf("mixed transient blocks should all be stripped, got %q", got)
	}
}
