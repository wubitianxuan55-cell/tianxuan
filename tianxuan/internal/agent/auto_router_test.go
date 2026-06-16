package agent

import "testing"

func TestAutoRouteComplexKeyword(t *testing.T) {
	tests := []struct {
		input    string
		expected AutoRouteModel
	}{
		{"refactor the auth module", AutoRoutePro},
		{"debug the null pointer", AutoRoutePro},
		{"implement a new API", AutoRoutePro},
		{"analyze the performance", AutoRoutePro},
		{"重构登录模块", AutoRoutePro},
		{"安全漏洞修复", AutoRoutePro},
		{"hi", AutoRouteFlash},
		{"what is go?", AutoRouteFlash},
		{"list files", AutoRouteFlash},
	}
	for _, tt := range tests {
		got := AutoRoute(tt.input)
		if got != tt.expected {
			t.Errorf("AutoRoute(%q) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestAutoRouteLengthHeuristic(t *testing.T) {
	// 短输入 → flash
	short := "fix typo"
	if got := AutoRoute(short); got != AutoRouteFlash {
		t.Errorf("short input should route to flash, got %s", got)
	}

	// 长输入 → pro
	long := ""
	for i := 0; i < 600; i++ {
		long += "x"
	}
	if got := AutoRoute(long); got != AutoRoutePro {
		t.Errorf("long input should route to pro, got %s", got)
	}
}

func TestAutoRouteDefaultFlash(t *testing.T) {
	// 普通中等长度输入 → flash
	medium := "please help me write a function that adds two numbers and returns the result"
	if got := AutoRoute(medium); got != AutoRouteFlash {
		t.Errorf("medium neutral input should default to flash, got %s", got)
	}
}
