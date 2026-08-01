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

// 工作流测试专用 mock：统计非 summarizer 请求次数，返回固定文本。
type workflowMock struct {
	t     *testing.T
	calls int
	text  string
}

func (m *workflowMock) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if isSummarizeRequest(body) {
		writeSSE(w, m.t,
			streamChunk(deltaText("summary")),
			finishChunk("stop"),
			usageChunk(100, 10, 0, 100),
		)
		return
	}
	m.calls++
	writeSSE(w, m.t,
		streamChunk(deltaText(m.text)),
		finishChunk("stop"),
		usageChunk(100, 10, 0, 100),
	)
}

func newWorkflowHermes(t *testing.T, plannerMock, execMock *workflowMock) (*Hermes, *Session, *Session) {
	t.Helper()
	srvP := httptest.NewServer(http.HandlerFunc(plannerMock.handler))
	t.Cleanup(srvP.Close)
	srvE := httptest.NewServer(http.HandlerFunc(execMock.handler))
	t.Cleanup(srvE.Close)

	plannerProv, err := openai.New(provider.Config{Name: "planner", BaseURL: srvP.URL, Model: "m", APIKey: "test"})
	if err != nil {
		t.Fatalf("planner provider: %v", err)
	}
	execProv, err := openai.New(provider.Config{Name: "executor", BaseURL: srvE.URL, Model: "m", APIKey: "test"})
	if err != nil {
		t.Fatalf("executor provider: %v", err)
	}

	plannerSess := NewSession("planner L1+L2")
	execSess := NewSession("executor L1+L2")
	hephaestus := New(execProv, tool.NewRegistry(), execSess, Options{
		MaxSteps:    5,
		Temperature: 0,
	}, event.Discard)
	h := NewHermes(plannerProv, plannerSess, nil, hephaestus, 0, event.Discard,
		tool.NewRegistry(), 1, 0, "", "")
	return h, plannerSess, execSess
}

func sessionContains(sess *Session, substr string) bool {
	for _, m := range sess.Snapshot() {
		if strings.Contains(m.Content, substr) {
			return true
		}
	}
	return false
}

// W1：auto-skip 直接执行的结果必须回灌规划者 session——否则用户下一轮说
// "继续" 时，规划者没有任何关于刚才直接执行工作的上下文。
func TestHermesDirectExecutionFeedsPlanner(t *testing.T) {
	plannerMock := &workflowMock{t: t, text: "<!--plan-->\n步骤 1：x"}
	execMock := &workflowMock{t: t, text: "Done."}
	h, plannerSess, _ := newWorkflowHermes(t, plannerMock, execMock)

	if _, err := h.Run(context.Background(), "fix typo in readme"); err != nil {
		t.Fatalf("Run(auto-skip): %v", err)
	}
	if plannerMock.calls != 0 {
		t.Fatalf("planner must not be called on auto-skip, got %d calls", plannerMock.calls)
	}
	if !sessionContains(plannerSess, "[上一轮执行结果]") {
		t.Fatal("W1: direct execution result must be fed into the planner session")
	}
}

// W1：! 快速路径同样回灌规划者。
func TestHermesFastPathFeedsPlanner(t *testing.T) {
	plannerMock := &workflowMock{t: t, text: "<!--plan-->\n步骤 1：x"}
	execMock := &workflowMock{t: t, text: "Done."}
	h, plannerSess, _ := newWorkflowHermes(t, plannerMock, execMock)

	if _, err := h.Run(context.Background(), "! 删除 main.go 中的空行"); err != nil {
		t.Fatalf("Run(fast path): %v", err)
	}
	if plannerMock.calls != 0 {
		t.Fatalf("planner must not be called on fast path, got %d calls", plannerMock.calls)
	}
	if !sessionContains(plannerSess, "[上一轮执行结果]") {
		t.Fatal("W1: fast-path result must be fed into the planner session")
	}
}

// W2：修正轮次的执行反馈必须带轮次标识，规划者才能区分原始执行与
// 第 N 轮修复执行，而不是看到多条格式相同的 [上一轮执行结果]。
func TestHermesFeedResultToPlanner_RoundMarker(t *testing.T) {
	plannerMock := &workflowMock{t: t, text: "plan"}
	execMock := &workflowMock{t: t, text: "Done."}
	h, plannerSess, _ := newWorkflowHermes(t, plannerMock, execMock)

	h.feedResultToPlanner(&TurnResult{Success: true, Summary: "fix applied"}, 2)
	if !sessionContains(plannerSess, "[第 2 轮修正执行结果]") {
		t.Fatal("W2: fix-round feedback must carry a round marker")
	}

	// round=1 保持原有格式（向后兼容）
	h.feedResultToPlanner(&TurnResult{Success: true, Summary: "original done"}, 1)
	msgs := plannerSess.Snapshot()
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Content, "[上一轮执行结果] success") {
		t.Fatalf("W2: round-1 feedback keeps the legacy marker, got: %q", last.Content)
	}
	if strings.Contains(last.Content, "[第 ") {
		t.Fatalf("W2: round-1 feedback must not carry a round marker, got: %q", last.Content)
	}
}
