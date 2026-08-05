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
