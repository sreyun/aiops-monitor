package main

import (
	"context"
	"strings"
	"testing"
)

// mockEvalLLM 是一个确定性 mock：对每个 case，把输入里的关键告警词映射到
// ground truth 关键词，返回一个「接近正确」的诊断。它用于验证评测框架本身
// 正确（case 加载、规则判定、通过率计算），而非评测 LLM 质量——后者由在线
// 评测（真实 provider）负责。
func mockEvalLLM(_ context.Context, system, user string) (string, error) {
	var b strings.Builder
	b.WriteString("可能根因分析：\n")
	// 根据 case 输入中的特征词给出含 ground truth 关键词的回答。
	table := []struct{ hint, root string }{
		{"Too many connections", "连接池 max_connections"},
		{"No space left", "磁盘满 WAL"},
		{"延迟从 2ms", "交换机广播风暴"},
		{"CrashLoopBackOff", "OOM 内存限制"},
		{"5xx 错误率", "后端线程池 超时"},
		{"inode 使用率", "inode 小文件"},
	}
	for _, e := range table {
		if strings.Contains(user, e.hint) {
			b.WriteString("根因：" + e.root + "\n")
		}
	}
	b.WriteString("排查动作：\n- 检查对应资源\n- 定位根因并处理\n")
	return b.String(), nil
}

func TestEvalCasesLoad(t *testing.T) {
	cases, err := loadEvalCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 5 {
		t.Fatalf("expected >=5 eval cases, got %d", len(cases))
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || c.Input == "" || len(c.GroundTruthRootCause) == 0 {
			t.Fatalf("case %s incomplete: %+v", c.ID, c)
		}
		if seen[c.ID] {
			t.Fatalf("duplicate case id %s", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestEvalRunSetOffline(t *testing.T) {
	ctx := context.Background()
	sum, err := runEvalSet(ctx, "mock-model", "offline", mockEvalLLM)
	if err != nil {
		t.Fatal(err)
	}
	if sum.CaseCount != lenMustMatch() {
		t.Fatalf("case count %d", sum.CaseCount)
	}
	// mock 对 6 个 case 都应能命中根因。
	if sum.PassRate < 0.5 {
		t.Fatalf("mock pass rate too low: %.2f (%d/%d)", sum.PassRate, sum.PassedCount, sum.CaseCount)
	}
	if sum.RootCauseHitRate < 0.5 {
		t.Fatalf("mock root cause hit too low: %.2f", sum.RootCauseHitRate)
	}
}

// lenMustMatch 用 loadEvalCases 的实际数量保证断言与评测集同步。
func lenMustMatch() int {
	cases, err := loadEvalCases()
	if err != nil {
		return -1
	}
	return len(cases)
}

func TestRuleJudge(t *testing.T) {
	pass, n := ruleJudge("连接池耗尽，max_connections 到顶", []evalRule{
		{Type: "keyword", Keywords: []string{"连接池", "max_connections"}, MinHit: 2},
		{Type: "keyword", Keywords: []string{"磁盘"}, MinHit: 1},
	})
	if pass != 1 || n != 2 {
		t.Fatalf("expected 1/2 rule pass, got %d/%d", pass, n)
	}
}

func TestFirstChinesePhrase(t *testing.T) {
	if got := firstChinesePhrase("检查 max_connections 配置"); got != "检查" {
		t.Fatalf("expected 检查, got %q", got)
	}
	if got := firstChinesePhrase("重启或扩容连接池"); got != "重启或扩" {
		t.Fatalf("expected 重启或扩, got %q", got)
	}
}
