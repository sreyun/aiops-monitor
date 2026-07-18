package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// ============================================================================
// Mixture-of-Agents（多模型集成研判）
//
// 高价值场景（诊断）下，用「主模型 + 额外模型」各出一份独立提案，再由主模型作为聚合器综合成
// 一份更稳、更少臆测的结论。借鉴 Nous Hermes Agent 的 MoA。默认关闭（cfg.MoAModels 为空），
// 开启后成本随参与模型数线性增长，故只用于诊断这类低频高价值调用。
// ============================================================================

// moaModelList 返回参与集成的模型列表 = [主模型, 额外模型...]（去重）。
// cfg.MoAModels 为逗号分隔的额外模型名；为空时列表仅含主模型（长度 1 = 未启用 MoA）。
func moaModelList(cfg AIConfig) []string {
	seen := map[string]bool{}
	var out []string
	if strings.TrimSpace(cfg.Model) != "" {
		seen[cfg.Model] = true
		out = append(out, cfg.Model)
	}
	for _, p := range strings.Split(cfg.MoAModels, ",") {
		if t := strings.TrimSpace(p); t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// aiChatMoAStream 集成研判：各模型并行出提案（非流式）→ 主模型聚合并【流式】输出（不发 [DONE]）。
// 未配置额外模型 / 有效提案不足 2 份时，退化为单模型 streamChatNoDone。返回最终全文。
func aiChatMoAStream(ctx context.Context, w http.ResponseWriter, cfg AIConfig, messages []map[string]string) string {
	models := moaModelList(cfg)
	if len(models) <= 1 { // 未启用 MoA
		out, _ := streamChatNoDone(ctx, w, cfg, messages, nil)
		return out
	}
	if f, ok := w.(http.Flusher); ok {
		fmt.Fprintf(w, "data: {\"reasoning\":%s}\n\n", jsonString(fmt.Sprintf("正在用 %d 个模型集成研判…", len(models))))
		f.Flush()
	}
	// 并行取各模型提案（非流式）
	type proposal struct{ model, text string }
	ch := make(chan proposal, len(models))
	for _, m := range models {
		go func(model string) {
			mc := cfg
			mc.Model = model
			txt, _, err := aiChatV(ctx, mc, messages, nil, nil)
			if err != nil {
				txt = ""
			}
			ch <- proposal{model, txt}
		}(m)
	}
	var proposals []proposal
	for range models {
		if p := <-ch; strings.TrimSpace(p.text) != "" {
			proposals = append(proposals, p)
		}
	}
	// 有效提案不足 2 份：无法体现集成价值，退化为单模型流式
	if len(proposals) < 2 {
		out, _ := streamChatNoDone(ctx, w, cfg, messages, nil)
		return out
	}
	// 聚合：把多份提案连同原始证据交主模型综合，流式输出
	var pb strings.Builder
	for i, p := range proposals {
		fmt.Fprintf(&pb, "【提案 %d · 模型 %s】\n%s\n\n", i+1, p.model, trimLine(p.text, 3000))
	}
	aggInstr := "以上是多个模型对同一问题给出的独立诊断提案。你作为首席 SRE，请对比它们，取长补短、" +
		"剔除臆测与自相矛盾之处，综合成【一份】最可靠的结论，并保持原诊断要求的结构。只依据提案与已知证据，不新增臆测。"
	aggMsgs := make([]map[string]string, 0, len(messages)+1)
	aggMsgs = append(aggMsgs, messages...) // 复用原始 system(含证据) + user(诊断要求)
	aggMsgs = append(aggMsgs, map[string]string{"role": "user", "content": pb.String() + "\n" + aggInstr})
	out, _ := streamChatNoDone(ctx, w, cfg, aggMsgs, nil)
	return out
}
