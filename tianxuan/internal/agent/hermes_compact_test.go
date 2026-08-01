package agent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/provider/openai"
	"tianxuan/internal/tool"
)

// O1 mock：规划者请求返回高 usage（触发 force compaction）与带 <!--plan-->
// 标记的计划文本；summarizer 请求返回简短摘要。summarizeCalls 用于证明
// 压缩确实发生过（否则断言是空洞的）。
type compactPlannerMock struct {
	t              *testing.T
	summarizeCalls int
}

func (m *compactPlannerMock) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if isSummarizeRequest(body) {
		m.summarizeCalls++
		writeSSE(w, m.t,
			streamChunk(deltaText("- goal: keep going\n- pending: none")),
			finishChunk("stop"),
			usageChunk(200, 20, 0, 200),
		)
		return
	}
	plan := "<!--plan-->\n步骤 1：写失败测试\n- **Verify**：go test ./...\n步骤 2：最小实现\n- **Verify**：go test ./..."
	writeSSE(w, m.t,
		streamChunk(deltaText(plan)),
		finishChunk("stop"),
		usageChunk(5000, 100, 0, 5000),
	)
}

// newCompactHermes 构造一个规划器 session 已越过压缩阈值的 Hermes：
// 预置 10 条不可 pin 的大历史消息（每条远超 pinned 预算），窗口 2000，
// 使 plannerAgent.Run 首轮即触发 force compaction。
func newCompactHermes(t *testing.T, mock *compactPlannerMock) (*Hermes, *Session) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	t.Cleanup(srv.Close)
	prov, err := openai.New(provider.Config{
		Name:    "deepseek",
		BaseURL: srv.URL,
		Model:   "deepseek-reasoner",
		APIKey:  "test",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	sess := NewSession("planner L1+L2")
	for i := 0; i < 10; i++ {
		sess.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: "[上一轮执行结果] " + strings.Repeat("详细反馈与文件变更记录。", 100),
		})
	}
	readOnlyReg := tool.NewRegistry()
	h := NewHermes(prov, sess, nil, nil, 0, event.Discard, readOnlyReg, 1, 2000, t.TempDir(), "")
	return h, sess
}

func sessionHasDigest(sess *Session) bool {
	for _, m := range sess.Snapshot() {
		if strings.Contains(m.Content, "<compaction-summary>") {
			return true
		}
	}
	return false
}

// TestHermesPlan_CompactionDigestSurvivesPlanTurn 验证 O1：规划期间发生的
// 压缩不能被 planWithConfirmation 的快照/恢复丢弃——否则持久 session 永不
// 缩减，每轮重复付出 summarizer 调用。
func TestHermesPlan_CompactionDigestSurvivesPlanTurn(t *testing.T) {
	mock := &compactPlannerMock{t: t}
	h, sess := newCompactHermes(t, mock)

	plan, err := h.planWithConfirmation(context.Background(), "实现一个功能")
	if err != nil {
		t.Fatalf("planWithConfirmation: %v", err)
	}
	if plan == nil {
		t.Fatal("expected a confirmed plan")
	}
	if mock.summarizeCalls == 0 {
		t.Fatal("test setup: compaction never fired; assertions are vacuous")
	}
	if !sessionHasDigest(sess) {
		t.Fatal("O1: planner compaction digest was discarded by snapshot/restore; session keeps growing")
	}
	// 本轮规划消息（用户输入）不应残留——规划消息仍是瞬态的
	for _, m := range sess.Snapshot() {
		if m.Content == "实现一个功能" {
			t.Fatal("planning input leaked into persistent planner session")
		}
	}
}

// TestHermesPlanFix_CompactionDigestSurvivesPlanTurn 验证 planFix 的
// 快照/恢复同样不能丢弃压缩 digest。
func TestHermesPlanFix_CompactionDigestSurvivesPlanTurn(t *testing.T) {
	mock := &compactPlannerMock{t: t}
	h, sess := newCompactHermes(t, mock)

	failed := &TurnResult{
		Plan:    "<!--plan-->\n步骤 1：x",
		Success: false,
		Errors:  []string{"build failed"},
	}
	fixPlan, err := h.planFix(context.Background(), "原始任务", "原计划", "", failed, 2, nil)
	if err != nil {
		t.Fatalf("planFix: %v", err)
	}
	if fixPlan == nil {
		t.Fatal("expected a fix plan")
	}
	if mock.summarizeCalls == 0 {
		t.Fatal("test setup: compaction never fired; assertions are vacuous")
	}
	if !sessionHasDigest(sess) {
		t.Fatal("O1: planFix discarded the compaction digest")
	}
}
