package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"tianxuan/internal/strutil"
	"tianxuan/internal/provider"
)

// verifyPrefix ������ stream() ʱ���� L1+L2+tools �� SHA256 ָ�ƣ�
// ����ÿ��У��ָ���Ƿ�ƥ�䡣Ư�� �� panic�����⾲Ĭ�ƻ� DeepSeek ǰ׺���档
// ���� Kun immutable-prefix.ts �� Go ��ֲ��
func (a *AgentRunner) verifyPrefix(msgs []provider.Message, tools []provider.ToolSchema) {
	// ��ȡ L1 �� L2 ����
	var l1, l2 string
	if len(msgs) > 0 && msgs[0].Role == provider.RoleSystem {
		l1 = msgs[0].Content
	}
	if len(msgs) > 1 && msgs[1].Role == provider.RoleSystem {
		l2 = msgs[1].Content
	}

	// �淶�������б������������
	sorted := make([]provider.ToolSchema, len(tools))
	copy(sorted, tools)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	// ����ָ��
	h := sha256.New()
	h.Write([]byte(l1))
	h.Write([]byte(l2))
	h.Write([]byte{0}) // �ָ��
	for _, t := range sorted {
		h.Write([]byte(t.Name))
		h.Write([]byte(t.Description))
		h.Write(t.Parameters)
		h.Write([]byte{0})
	}
	fp := hex.EncodeToString(h.Sum(nil))[:16]

	if !a.prefixFingerprintSet {
		a.prefixFingerprint = fp
		a.prefixFingerprintSet = true
		return
	}
	if a.prefixFingerprint != fp {
		panic(fmt.Sprintf(
			"ImmutablePrefix DRIFT: L1/L2/tools fingerprint changed mid-session!\n"+
				"  expected: %s\n  actual:   %s\n"+
				"This will silently break DeepSeek prefix cache on every turn.\n"+
				"Check: is any code path mutating the system prompt, runtime prompt, or tool schemas after Lock()?",
			a.prefixFingerprint, fp))
	}
}

// cacheBreakDetector ��ÿ�� API ����ǰ��Ա� L1/L2/tools ��ϣ�� cache_read��
// �������������½� >5% �� >2000 tokens ʱ��϶���ԭ��
// ���������������޸��κλ���ǰ׺����Ӱ�� L1/L2/tools �ȶ��ԡ�
type cacheBreakDetector struct {
	prevL1    uint64 // 上上次 L1 哈希（用于 diagnose 比对）
	prevL2    uint64 // 上上次 L2 哈希
	prevTools uint64 // 上上次 tools 哈希
	lastL1    uint64 // �ϴ� L1 system ��Ϣ�� FNV-1a ��ϣ
	lastL2    uint64 // �ϴ� L2 runtime ��Ϣ�� FNV-1a ��ϣ
	lastTools uint64 // �ϴι��� schemas �� FNV-1a ��ϣ
	lastRead  int    // �ϴ� cache_read ֵ
	callCount int    // ���ü���
	prevMsgCount int    // L4 message count (prev)
	lastMsgCount int    // L4 message count (last)
	prevPrefixHash uint64 // 全部非系统消息哈希 (prev)
	lastPrefixHash uint64 // 全部非系统消息哈希 (last)
}

// record �� API ����ǰ��¼��ǰǰ׺��ϣ��
func (d *cacheBreakDetector) record(msgs []provider.Message, tools []provider.ToolSchema) {
	var l1, l2 string
	if len(msgs) > 0 && msgs[0].Role == provider.RoleSystem {
		l1 = msgs[0].Content
	}
	if len(msgs) > 1 && msgs[1].Role == provider.RoleSystem {
		l2 = msgs[1].Content
	}
	// 保存旧哈希用于 diagnose 比对
	d.prevL1 = d.lastL1
	d.prevL2 = d.lastL2
	d.prevTools = d.lastTools
	d.prevMsgCount = d.lastMsgCount
	d.prevPrefixHash = d.lastPrefixHash

	d.lastL1 = fnv1a(l1)
	d.lastL2 = fnv1a(l2)
	d.lastTools = hashTools(tools)
	// L4: 全量非系统消息累积异或哈希。diagnose 仅在消息数未增长时比对
	// ——正常增长跳过不报警，但等量消息内的 tool_result 变化会检出。
	d.lastMsgCount = len(msgs)
	var h uint64
	for _, m := range msgs {
		if m.Role != provider.RoleSystem {
			h ^= fnv1a(string(m.Role) + ":" + truncateStr(m.Content, 80))
		}
	}
	h ^= uint64(d.lastMsgCount) * 0x9E3779B97F4A7C15
	d.lastPrefixHash = h
	d.callCount++
}

// check �� API ���ú��⻺����ѡ�
// ���ؿ��ַ�����ʾ�������ǿձ�ʾ����ԭ��
func (d *cacheBreakDetector) check(u *provider.Usage) string {
	if d.lastRead == 0 {
		d.lastRead = u.CacheHitTokens
		return "" // �״ε��ã��޻���
	}

	drop := d.lastRead - u.CacheHitTokens
	threshold := int(float64(d.lastRead) * 0.05)
	if threshold < 2000 {
		threshold = 2000
	}
	if drop < threshold {
		d.lastRead = u.CacheHitTokens
		return "" // ��������
	}

	// 诊断断裂原因
	reason := d.diagnose()
	d.lastRead = u.CacheHitTokens
	return "[cache break #" + strutil.Itoa(d.callCount) + ": " +
		strutil.Itoa(d.lastRead+drop) + "→" + strutil.Itoa(u.CacheHitTokens) +
		" tok (" + reason + ")]"
}

// diagnose ����������ѵĿ���ԭ��
func (d *cacheBreakDetector) diagnose() string {
	var parts []string
	if d.prevL1 != d.lastL1 {
		parts = append(parts, "L1 changed")
	}
	if d.prevL2 != d.lastL2 {
		parts = append(parts, "L2 changed")
	}
	if d.prevTools != d.lastTools {
		parts = append(parts, "tools changed")
	}
	if d.lastMsgCount < d.prevMsgCount {
		parts = append(parts, fmt.Sprintf("messages truncated %d→%d", d.prevMsgCount, d.lastMsgCount))
	} else if d.lastMsgCount == d.prevMsgCount && d.prevMsgCount > 0 && d.prevPrefixHash != d.lastPrefixHash {
		parts = append(parts, fmt.Sprintf("L4 content changed (%d msgs)", d.lastMsgCount))
	}
	if len(parts) > 0 {
		return "client-side prefix drift: " + strings.Join(parts, ", ")
	}
	return "server-side (L1/L2/tools/L4 stable)"
}


func fnv1a(s string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}

// hashTools ���㹤�� schemas ����Ϲ�ϣ��
func hashTools(tools []provider.ToolSchema) uint64 {
	h := fnv1a(strutil.Itoa(len(tools)))
	for _, t := range tools {
		h ^= fnv1a(t.Name)
		h ^= fnv1a(t.Description)
	}
	return h
}
