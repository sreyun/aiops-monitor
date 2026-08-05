package main

import (
	"encoding/json"
	"strings"
)

// resolveModelForTask picks a model for an assist/chat task.
// Priority: TaskModelsJSON map → CheapModel for light tasks → cfg.Model.
// routed=true when the chosen model differs from the configured primary Model.
func resolveModelForTask(cfg AIConfig, task string) (model string, routed bool) {
	primary := strings.TrimSpace(cfg.Model)
	task = strings.ToLower(strings.TrimSpace(task))
	chosen := primary
	if m := taskModelFromJSON(cfg.TaskModelsJSON, task); m != "" {
		chosen = m
	} else if cheap := strings.TrimSpace(cfg.CheapModel); cheap != "" && isCheapAITask(task) {
		chosen = cheap
	}
	if chosen == "" {
		chosen = primary
	}
	return chosen, chosen != "" && chosen != primary
}

func taskModelFromJSON(raw, task string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || task == "" {
		return ""
	}
	var m map[string]string
	if json.Unmarshal([]byte(raw), &m) != nil {
		return ""
	}
	if v := strings.TrimSpace(m[task]); v != "" {
		return v
	}
	for k, v := range m {
		if strings.EqualFold(k, task) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func isCheapAITask(task string) bool {
	switch task {
	case "promql", "logql", "pgsql", "sqlql", "sql_beautify", "dashboard_prompt_optimize",
		"summarize", "summary", "translate", "legend", "title", "rename", "classify",
		"tag", "label", "format", "beautify":
		return true
	default:
		return strings.HasSuffix(task, "_suggest") || strings.HasPrefix(task, "fmt_")
	}
}

func applyRoutedModel(cfg AIConfig, task string) AIConfig {
	if m, _ := resolveModelForTask(cfg, task); m != "" {
		cfg.Model = m
	}
	return cfg
}

// ModelPrice 是单个模型每百万 token 的价格（成本估算护栏用）。
type ModelPrice struct {
	InputPer1M  float64 `json:"input_per_1m"`
	OutputPer1M float64 `json:"output_per_1m"`
}

// ModelRouteDecision 是一次任务级模型路由决策的结果，附带成本单价。
type ModelRouteDecision struct {
	Task        string
	Model       string
	Routed      bool
	InputPer1M  float64
	OutputPer1M float64
	Reason      string
}

// resolveModelRoute 基于任务解析模型路由决策，并附带成本单价。
// 价格来源：ModelPricingJSON 命中的模型取配置单价，未配置时回退主配置单价。
func resolveModelRoute(cfg AIConfig, task string) ModelRouteDecision {
	model, routed := resolveModelForTask(cfg, task)
	d := ModelRouteDecision{Task: task, Model: model, Routed: routed}
	if price := modelPriceFromJSON(cfg.ModelPricingJSON, model); price != nil {
		d.InputPer1M, d.OutputPer1M = price.InputPer1M, price.OutputPer1M
	} else {
		d.InputPer1M, d.OutputPer1M = cfg.InputPricePer1M, cfg.OutputPricePer1M
	}
	switch {
	case model == "":
		d.Reason = "no_model"
	case routed && taskModelFromJSON(cfg.TaskModelsJSON, task) != "":
		d.Reason = "task_models"
	case routed:
		d.Reason = "cheap_model"
	default:
		d.Reason = "primary"
	}
	return d
}

// modelPriceFromJSON 读取 ModelPricingJSON 中指定模型的单价；未配置返回 nil。
func modelPriceFromJSON(raw, model string) *ModelPrice {
	raw = strings.TrimSpace(raw)
	model = strings.TrimSpace(model)
	if raw == "" || model == "" {
		return nil
	}
	var m map[string]ModelPrice
	if json.Unmarshal([]byte(raw), &m) != nil {
		return nil
	}
	if p, ok := m[model]; ok {
		return &p
	}
	for k, v := range m {
		if strings.EqualFold(k, model) {
			p := v
			return &p
		}
	}
	return nil
}

// routeReason values recorded in ai_call_events.route_reason.
const (
	routeReasonTaskModels = "task_models"
	routeReasonCheapModel = "cheap_model"
	routeReasonPrimary    = "primary"
	routeReasonFallback   = "fallback"
	routeReasonUnknown    = "unknown"
)

// inferRouteReason 在落账本时推断一次 AI 调用「为什么用了 usedModel」。
// 判定顺序与 resolveModelForTask 完全一致（TaskModelsJSON → CheapModel → primary），
// 最后兜底 fallback：若 usedModel 不属于任何路由选择，则说明是故障转移换上的模型。
// 之所以在 recordAICallActor 推断而非在决策处透传，是因为 fallback 发生在更深的
// 调用链（aiChatVWithFallback），只有最终 usedModel 才能准确反映实际执行。
func inferRouteReason(cfg AIConfig, task, usedModel string) string {
	usedModel = strings.TrimSpace(usedModel)
	if usedModel == "" {
		return routeReasonUnknown
	}
	if m := taskModelFromJSON(cfg.TaskModelsJSON, task); m != "" {
		if strings.EqualFold(strings.TrimSpace(m), usedModel) {
			return routeReasonTaskModels
		}
	}
	if cheap := strings.TrimSpace(cfg.CheapModel); cheap != "" && isCheapAITask(task) {
		if strings.EqualFold(cheap, usedModel) {
			return routeReasonCheapModel
		}
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Model), usedModel) {
		return routeReasonPrimary
	}
	return routeReasonFallback
}

// EstimateQueryCost 按输入/输出 token 估算一次调用的成本（元）。
func (d ModelRouteDecision) EstimateQueryCost(inTokens, outTokens int) float64 {
	return (float64(inTokens)/1e6)*d.InputPer1M + (float64(outTokens)/1e6)*d.OutputPer1M
}

// costGuardrailOK 判断单次调用估算成本是否超过护栏上限（MaxCostPerQueryCNY<=0 表示不限制）。
func costGuardrailOK(cfg AIConfig, task string, inTokens, outTokens int) bool {
	if cfg.MaxCostPerQueryCNY <= 0 {
		return true
	}
	d := resolveModelRoute(cfg, task)
	return d.EstimateQueryCost(inTokens, outTokens) <= cfg.MaxCostPerQueryCNY
}
