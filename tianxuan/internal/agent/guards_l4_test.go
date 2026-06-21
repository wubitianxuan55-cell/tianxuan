package agent

import (
	"strings"
	"testing"

	"tianxuan/internal/provider"
)

func msgTool(name, content string) provider.Message {
	return provider.Message{Role: provider.RoleTool, Content: content, Name: name}
}

// TestCacheBreakDetectorMessagesTruncated 检测消息截断 —— compaction 后
// 消息数减少应诊断为 "messages truncated"。
func TestCacheBreakDetectorMessagesTruncated(t *testing.T) {
	var d cacheBreakDetector

	// 模拟截断前：10 条消息（2 条 system + 8 条非 system）
	msgsBefore := []provider.Message{
		msgSys("L1"), msgSys("L2"),
		msgUser("q1"), msgTool("t1", "result1"), msgUser("q2"),
		msgTool("t2", "result2"), msgUser("q3"), msgTool("t3", "result3"),
		msgUser("q4"), msgTool("t4", "result4"),
	}
	tools := toolsForTest("a", "b")

	d.record(msgsBefore, tools)
	d.check(&provider.Usage{CacheHitTokens: 10000})

	// 模拟 compaction 截断后：仅 5 条消息（2 system + 3 非 system）
	msgsAfter := []provider.Message{
		msgSys("L1"), msgSys("L2"),
		msgUser("[compaction summary]"),
		msgUser("q4"), msgTool("t4", "result4"),
	}
	d.record(msgsAfter, tools)
	reason := d.check(&provider.Usage{CacheHitTokens: 500}) // -95%，远超 2000 token 阈值
	if !strings.Contains(reason, "client-side") {
		t.Errorf("truncation should be detected as client-side drift, got %q", reason)
	}
	if !strings.Contains(reason, "messages truncated") {
		t.Errorf("expected 'messages truncated' in reason, got %q", reason)
	}
}

// TestCacheBreakDetectorL4ContentChanged 检测 L4 消息内容变化 ——
// 消息数相同但某个 tool_result 内容被替换时应报警。
func TestCacheBreakDetectorL4ContentChanged(t *testing.T) {
	var d cacheBreakDetector

	msgs1 := []provider.Message{
		msgSys("L1"), msgSys("L2"),
		msgUser("q1"), msgTool("echo", "result_abc"), msgUser("q2"),
	}
	msgs2 := []provider.Message{
		msgSys("L1"), msgSys("L2"),
		msgUser("q1"), msgTool("echo", "result_xyz"), msgUser("q2"),
	}
	tools := toolsForTest("a", "b")

	d.record(msgs1, tools)
	d.check(&provider.Usage{CacheHitTokens: 10000})

	d.record(msgs2, tools)
	reason := d.check(&provider.Usage{CacheHitTokens: 500}) // -95%
	if !strings.Contains(reason, "client-side") {
		t.Errorf("L4 content change should be client-side drift, got %q", reason)
	}
	if !strings.Contains(reason, "L4 content changed") {
		t.Errorf("expected 'L4 content changed' in reason, got %q", reason)
	}
}

// TestCacheBreakDetectorNormalGrowth 正常消息增长不应触发 L4 检测
// —— 消息数递增是正常行为，不应误报。即使缓存命中因服务端原因
// 大幅下降，diagnose 也应识别为 server-side。
func TestCacheBreakDetectorNormalGrowth(t *testing.T) {
	var d cacheBreakDetector

	msgs1 := []provider.Message{
		msgSys("L1"), msgSys("L2"), msgUser("q1"),
	}
	msgs2 := []provider.Message{
		msgSys("L1"), msgSys("L2"), msgUser("q1"),
		msgTool("echo", "ok"), msgUser("q2"),
	}
	msgs3 := []provider.Message{
		msgSys("L1"), msgSys("L2"), msgUser("q1"),
		msgTool("echo", "ok"), msgUser("q2"),
		msgTool("echo2", "ok2"), msgUser("q3"),
	}
	tools := toolsForTest("a", "b")

	d.record(msgs1, tools)
	d.check(&provider.Usage{CacheHitTokens: 10000})

	d.record(msgs2, tools)
	d.check(&provider.Usage{CacheHitTokens: 9800}) // 正常波动

	d.record(msgs3, tools)
	// 模拟服务端缓存 TTL 过期导致大幅下降，但前缀稳定
	reason := d.check(&provider.Usage{CacheHitTokens: 500}) // -95%
	if !strings.Contains(reason, "server-side") {
		t.Errorf("normal growth should not trigger client-side drift, got %q", reason)
	}
	if strings.Contains(reason, "messages truncated") || strings.Contains(reason, "L4 content changed") {
		t.Errorf("normal growth should not trigger L4 diagnostics, got %q", reason)
	}
}
