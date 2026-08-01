package planmode

import "testing"

// V10.134 契约：单模型 auto-plan 模式下，Marker 必须指引模型在输出计划
// 后用 ask 工具请求用户批准（宿主据此切换出只读计划门），否则计划模式
// 只能调查、无法衔接执行。
func TestMarker_GuidesAskApproval(t *testing.T) {
	for _, kw := range []string{"ask", "提交执行", "按意见修改", "取消", "plan"} {
		if !containsAny(Marker, kw) {
			t.Errorf("Marker should contain %q (ask-approval workflow)", kw)
		}
	}
}

func containsAny(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
