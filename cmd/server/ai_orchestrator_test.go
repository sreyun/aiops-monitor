package main

import (
	"strings"
	"testing"
	"time"
)

// AI 黄金集（P2-3）：不调真实 LLM，校验任务路由策略与关键 system prompt 契约。
// CI 烟雾用：go test ./cmd/server/ -count=1 -run 'TestAIGolden|TestAIStats|TestAssistTaskPolicy'

type goldenCase struct {
	name           string
	task           string
	wantMemKind    string
	wantNoThink    bool
	wantTimeout    bool // Timeout > 0
	promptMustHave []string
	promptForbid   []string
}

func TestAssistTaskPolicy_Golden(t *testing.T) {
	cases := []goldenCase{
		{name: "logql", task: "logql", wantMemKind: "chat", wantTimeout: true, promptMustHave: []string{"LogQL"}},
		{name: "promql", task: "promql", wantMemKind: "chat", wantTimeout: true, promptMustHave: []string{"PromQL"}},
		{name: "playbook", task: "playbook", wantMemKind: "chat", wantTimeout: true, promptMustHave: []string{"剧本"}},
		{name: "chart", task: "chart_analysis", wantMemKind: "diagnosis", promptMustHave: []string{"SRE"}},
		{name: "hw", task: "hardware_diagnosis", wantMemKind: "diagnosis", promptMustHave: []string{"硬件"}},
		{name: "hyperv", task: "hyperv_diagnosis", wantMemKind: "diagnosis", promptMustHave: []string{"Hyper-V"}},
		{name: "snmp", task: "snmp_diagnosis", wantMemKind: "diagnosis", promptMustHave: []string{"SNMP"}},
		{name: "trap", task: "trap_diagnosis", wantMemKind: "diagnosis", promptMustHave: []string{"Trap"}},
		{name: "netflow", task: "netflow_diagnosis", wantMemKind: "diagnosis", promptMustHave: []string{"流量"}},
		{name: "checks", task: "checks_diagnosis", wantMemKind: "diagnosis", promptMustHave: []string{"拨测"}},
		{name: "forward", task: "forward_diagnosis", wantMemKind: "diagnosis", promptMustHave: []string{"转发"}},
		{name: "apimon", task: "apimon_diagnosis", wantMemKind: "diagnosis", promptMustHave: []string{"API"}},
		{name: "audit", task: "audit_diagnosis", wantMemKind: "diagnosis"},
		{name: "content", task: "content_audit_diagnosis", wantMemKind: "diagnosis", promptMustHave: []string{"敏感"}},
		{name: "dash_opt", task: "dashboard_optimize", wantMemKind: "diagnosis", wantNoThink: true, wantTimeout: true, promptMustHave: []string{"看板"}},
		{name: "dash_ana", task: "dashboard_analysis", wantMemKind: "diagnosis", wantNoThink: true, wantTimeout: true},
		{name: "dash_prompt", task: "dashboard_prompt_optimize", wantMemKind: "chat", wantNoThink: true, wantTimeout: true, promptMustHave: []string{"看板"}},
		{name: "remediation", task: "remediation_rule", wantMemKind: "chat", wantTimeout: true, promptMustHave: []string{"规则"}},
		{name: "duty", task: "duty_report", wantMemKind: "chat", promptMustHave: []string{"值班"}},
		{name: "generic", task: "generic", wantMemKind: "chat"},
	}
	if len(cases) < 20 {
		t.Fatalf("golden set too small: %d", len(cases))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := assistTaskPolicy(c.task)
			if p.MemKind != c.wantMemKind {
				t.Errorf("MemKind=%q want %q", p.MemKind, c.wantMemKind)
			}
			if p.DisableThink != c.wantNoThink {
				t.Errorf("DisableThink=%v want %v", p.DisableThink, c.wantNoThink)
			}
			if c.wantTimeout && p.Timeout <= 0 {
				t.Errorf("expected Timeout > 0")
			}
			if !c.wantTimeout && p.Timeout != 0 && c.task != "dashboard_prompt_optimize" {
				// 仅标注 wantTimeout 的任务强制；其它允许 0
			}
			if p.RememberSource != "assist:"+c.task {
				t.Errorf("RememberSource=%q", p.RememberSource)
			}
			sys := buildAssistSystemPrompt(c.task, "【测试上下文】host=demo")
			if !strings.Contains(sys, "【测试上下文】") {
				t.Errorf("context not injected")
			}
			for _, must := range c.promptMustHave {
				if !strings.Contains(sys, must) {
					t.Errorf("prompt missing %q; got prefix %q", must, trimLine(sys, 80))
				}
			}
			for _, forbid := range c.promptForbid {
				if strings.Contains(sys, forbid) {
					t.Errorf("prompt unexpectedly contains %q", forbid)
				}
			}
		})
	}
}

func TestAIGolden_HyperVAndHardwarePromptsDistinct(t *testing.T) {
	hw := buildAssistSystemPrompt("hardware_diagnosis", "")
	hv := buildAssistSystemPrompt("hyperv_diagnosis", "")
	if !strings.Contains(hw, "硬件") {
		t.Fatal("hardware prompt")
	}
	if !strings.Contains(hv, "Hyper-V") {
		t.Fatal("hyperv prompt")
	}
	if hw == hv {
		t.Fatal("prompts should differ")
	}
}

func TestAIStatsHub_RecordAndSnapshot(t *testing.T) {
	h := newAIStatsHub()
	h.record(aiCallStat{Ts: time.Now().Unix(), Task: "logql", Model: "m", LatencyMs: 100, OK: true, ApproxTokens: 50})
	h.record(aiCallStat{Ts: time.Now().Unix(), Task: "logql", Model: "m", LatencyMs: 200, OK: false, Error: "boom", ApproxTokens: 10})
	h.record(aiCallStat{Ts: time.Now().Unix(), Task: "chat", Model: "m", LatencyMs: 50, OK: true, ApproxTokens: 20})
	snap := h.snapshot()
	if snap["total"].(int64) != 3 {
		t.Fatalf("total=%v", snap["total"])
	}
	if snap["fail"].(int64) != 1 {
		t.Fatalf("fail=%v", snap["fail"])
	}
	if snap["approx_tokens_total"].(int64) != 80 {
		t.Fatalf("tokens=%v", snap["approx_tokens_total"])
	}
	by := snap["by_task"].(map[string]aiTaskAgg)
	if by["logql"].Count != 2 || by["logql"].Fail != 1 {
		t.Fatalf("logql agg=%+v", by["logql"])
	}
	if by["logql"].AvgMs != 150 {
		t.Fatalf("avg=%d", by["logql"].AvgMs)
	}
}

func TestEstimateTokens_Smoke(t *testing.T) {
	if estimateTokens("") != 0 {
		t.Fatal("empty")
	}
	n := estimateTokens("你好世界 hello world")
	if n <= 0 {
		t.Fatal("expected positive estimate")
	}
}
