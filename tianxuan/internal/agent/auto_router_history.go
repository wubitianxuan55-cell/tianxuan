package agent

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
)

// ─── V7.4: 历史感知的模型路由 ──────────────────────────────────────────
//
// 在启发式关键词+长度路由的基础上，增加对历史调用结果的跟踪。
// 每个任务被指纹化为一个"桶"，桶内记录 flash/pro 各自的调用次数和失败次数。
// 如果 flash 在过去相似任务上的成功率 > 90%，优先走 flash；否则走 pro。
// 路由决策在会话内持续学习，不跨会话持久化。

// routeEntry 记录一个任务桶的调用历史。
type routeEntry struct {
	FlashCalls int     `json:"flash_calls"`
	FlashErrs  int     `json:"flash_errs"`
	ProCalls   int     `json:"pro_calls"`
	ProErrs    int     `json:"pro_errs"`
}

// routeBucketKey 从用户输入生成任务桶指纹。
// 使用输入的前 200 字符的关键词哈希，所以同类任务的指纹接近。
func routeBucketKey(input string) string {
	// 取前 200 字符并转为小写
	short := input
	if len(short) > 200 {
		short = short[:200]
	}
	lower := strings.ToLower(short)

	// 提取长度区间（100字符以内、100-500、500+）
	lenTag := "M"
	if len(input) < 100 {
		lenTag = "S"
	} else if len(input) > 500 {
		lenTag = "L"
	}

	// 提取关键词信号：前 5 个出现的关键词
	var keywords []string
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			keywords = append(keywords, kw)
			if len(keywords) >= 5 {
				break
			}
		}
	}

	// 生成稳定指纹
	base := lenTag + ":" + strings.Join(keywords, ",")
	h := sha256.Sum256([]byte(base))
	return fmt.Sprintf("%x", h[:4])
}

// RouteHistory 管理会话内的路由学习。
type RouteHistory struct {
	mu     sync.Mutex
	table  map[string]*routeEntry // bucket → stats

	// 配置
	MinSamples      int     // 闪存模式所需的最小样本数（默认 3）
	FlashThreshold  float64 // 闪存模式的最大失败率（默认 0.1 = 90% 成功率）
	BackoffAfterErr int     // 连续 Flash 错误后的回退次数（默认 1）
}

// NewRouteHistory 创建路由历史追踪器。
func NewRouteHistory() *RouteHistory {
	return &RouteHistory{
		table:           make(map[string]*routeEntry),
		MinSamples:      3,
		FlashThreshold:  0.1,
		BackoffAfterErr: 1,
	}
}

// Record 记录一次路由调用的结果。
// model 是 "flash" 或 "pro"。hasError 表示调用是否失败。
func (h *RouteHistory) Record(input string, model string, hasError bool) {
	bucket := routeBucketKey(input)
	h.mu.Lock()
	defer h.mu.Unlock()

	entry, ok := h.table[bucket]
	if !ok {
		entry = &routeEntry{}
		h.table[bucket] = entry
	}

	if model == "flash" || model == "deepseek-v4-flash" {
		entry.FlashCalls++
		if hasError {
			entry.FlashErrs++
		}
	} else {
		entry.ProCalls++
		if hasError {
			entry.ProErrs++
		}
	}
}

// ShouldRouteToFlash 判断是否应该将输入路由到 flash 模型。
// 返回 true = 走 flash，false = 走 pro。
// 当样本不足时回退到启发式决策。
func (h *RouteHistory) ShouldRouteToFlash(input string, heuristicPrefersFlash bool) bool {
	bucket := routeBucketKey(input)
	h.mu.Lock()
	entry, ok := h.table[bucket]
	h.mu.Unlock()

	if !ok {
		// 无历史：遵循启发式
		return heuristicPrefersFlash
	}

	totalFlash := entry.FlashCalls
	if totalFlash < h.MinSamples {
		// 样本不足：遵循启发式
		return heuristicPrefersFlash
	}

	// 有足够样本：基于历史错误率决策
	flashErrRate := float64(entry.FlashErrs) / float64(totalFlash)
	return flashErrRate <= h.FlashThreshold
}

// Stats 返回当前路由统计信息的可读字符串。用于诊断/调试。
func (h *RouteHistory) Stats() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.table) == 0 {
		return "route history: no data"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("route history: %d bucket(s)\n", len(h.table)))
	for bucket, entry := range h.table {
		flashRate := 0.0
		if entry.FlashCalls > 0 {
			flashRate = 1.0 - float64(entry.FlashErrs)/float64(entry.FlashCalls)
		}
		proRate := 0.0
		if entry.ProCalls > 0 {
			proRate = 1.0 - float64(entry.ProErrs)/float64(entry.ProCalls)
		}
		b.WriteString(fmt.Sprintf("  [%s] flash=%d(%.0f%%) pro=%d(%.0f%%)\n",
			bucket[:8], entry.FlashCalls, flashRate*100, entry.ProCalls, proRate*100))
	}
	return strings.TrimRight(b.String(), "\n")
}
