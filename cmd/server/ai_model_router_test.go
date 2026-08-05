package main

import "testing"

func TestResolveModelRouteTaskMapping(t *testing.T) {
	cfg := AIConfig{
		Model:            "gpt-4o",
		TaskModelsJSON:   `{"promql":"qwen-turbo","chart_analysis":"qwen-plus"}`,
		ModelPricingJSON: `{"qwen-turbo":{"input_per_1m":0.3,"output_per_1m":0.6},"gpt-4o":{"input_per_1m":2.5,"output_per_1m":10}}`,
	}
	d := resolveModelRoute(cfg, "promql")
	if d.Model != "qwen-turbo" || !d.Routed {
		t.Fatalf("promql should route to qwen-turbo, got %+v", d)
	}
	if d.InputPer1M != 0.3 || d.OutputPer1M != 0.6 {
		t.Fatalf("price should come from ModelPricingJSON, got %+v", d)
	}
	if d.Reason != "task_models" {
		t.Fatalf("reason should be task_models, got %s", d.Reason)
	}

	d2 := resolveModelRoute(cfg, "chart_analysis")
	if d2.Model != "qwen-plus" || !d2.Routed {
		t.Fatalf("chart_analysis should route to qwen-plus, got %+v", d2)
	}
}

func TestResolveModelRouteCheapFallback(t *testing.T) {
	cfg := AIConfig{Model: "gpt-4o", CheapModel: "gpt-4o-mini"}
	d := resolveModelRoute(cfg, "summarize")
	if d.Model != "gpt-4o-mini" || !d.Routed {
		t.Fatalf("summarize should route to cheap model, got %+v", d)
	}
	if d.Reason != "cheap_model" {
		t.Fatalf("reason should be cheap_model, got %s", d.Reason)
	}
}

func TestEstimateQueryCostAndGuardrail(t *testing.T) {
	cfg := AIConfig{
		Model:              "gpt-4o",
		InputPricePer1M:    2.5,
		OutputPricePer1M:   10,
		MaxCostPerQueryCNY: 0.1,
	}
	d := resolveModelRoute(cfg, "diagnose")
	// 10k in + 2k out -> 0.025 + 0.02 = 0.045 <= 0.1
	if !costGuardrailOK(cfg, "diagnose", 10000, 2000) {
		t.Fatal("cost 0.045 should be under guardrail 0.1")
	}
	// 50k in + 20k out -> 0.125 + 0.2 = 0.325 > 0.1
	if costGuardrailOK(cfg, "diagnose", 50000, 20000) {
		t.Fatal("cost 0.325 should exceed guardrail 0.1")
	}
	if d.EstimateQueryCost(10000, 2000) != 0.045 {
		t.Fatalf("unexpected cost: %v", d.EstimateQueryCost(10000, 2000))
	}
	// 未配置护栏时永远放行
	cfg.MaxCostPerQueryCNY = 0
	if !costGuardrailOK(cfg, "diagnose", 1e9, 1e9) {
		t.Fatal("no guardrail should allow")
	}
}

func TestInferRouteReason(t *testing.T) {
	cfg := AIConfig{
		Model:          "gpt-4o",
		CheapModel:     "gpt-4o-mini",
		TaskModelsJSON: `{"promql":"qwen-turbo"}`,
	}
	cases := []struct {
		task, model, want string
	}{
		// TaskModelsJSON 优先
		{"promql", "qwen-turbo", routeReasonTaskModels},
		// Cheap task 走 cheap model
		{"summarize", "gpt-4o-mini", routeReasonCheapModel},
		// 主模型
		{"diagnose", "gpt-4o", routeReasonPrimary},
		// 不在任何路由选择 → fallback
		{"diagnose", "claude-sonnet", routeReasonFallback},
		// 空模型 → unknown
		{"diagnose", "", routeReasonUnknown},
	}
	for _, c := range cases {
		if got := inferRouteReason(cfg, c.task, c.model); got != c.want {
			t.Errorf("inferRouteReason(%q,%q) = %q, want %q", c.task, c.model, got, c.want)
		}
	}
}

// TestInferRouteReasonMatchesRoute：推断结果必须与 resolveModelForTask 的实际路由一致
// （对非 fallback 的 usedModel），保证账本里的 reason 反映真实路由。
func TestInferRouteReasonMatchesRoute(t *testing.T) {
	cfg := AIConfig{
		Model:          "gpt-4o",
		CheapModel:     "gpt-4o-mini",
		TaskModelsJSON: `{"promql":"qwen-turbo","summarize":"qwen-turbo"}`,
	}
	for _, task := range []string{"promql", "summarize", "chart_analysis", "diagnose", "translate"} {
		routed, _ := resolveModelForTask(cfg, task)
		got := inferRouteReason(cfg, task, routed)
		if got == routeReasonFallback || got == routeReasonUnknown {
			t.Errorf("task %q routed to %q but reason=%q (should be task_models/cheap/primary)", task, routed, got)
		}
	}
}
