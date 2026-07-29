package main

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
)

// Per-call usage slots avoid global mutex cross-talk under concurrent AI calls.
type aiUsageSlot struct {
	prompt, completion int
	set                bool
}

type aiUsageCtxKey struct{}

var (
	aiUsageSeq   uint64
	aiUsageSlots sync.Map // uint64 -> *aiUsageSlot
	// legacy fallback for call paths that do not bind a slot
	aiUsageMu   sync.Mutex
	aiUsageLast struct {
		prompt, completion int
		set                bool
	}
)

func withAIUsageSlot(ctx context.Context) (context.Context, uint64) {
	if ctx == nil {
		ctx = context.Background()
	}
	id := atomic.AddUint64(&aiUsageSeq, 1)
	aiUsageSlots.Store(id, &aiUsageSlot{})
	return context.WithValue(ctx, aiUsageCtxKey{}, id), id
}

func endAIUsageSlot(id uint64) {
	aiUsageSlots.Delete(id)
}

func captureAIUsage(promptTok, completionTok int) {
	captureAIUsageCtx(context.Background(), promptTok, completionTok)
}

func captureAIUsageCtx(ctx context.Context, promptTok, completionTok int) {
	if promptTok <= 0 && completionTok <= 0 {
		return
	}
	if ctx != nil {
		if id, ok := ctx.Value(aiUsageCtxKey{}).(uint64); ok {
			if v, ok2 := aiUsageSlots.Load(id); ok2 {
				slot := v.(*aiUsageSlot)
				slot.prompt = promptTok
				slot.completion = completionTok
				slot.set = true
				return
			}
		}
	}
	aiUsageMu.Lock()
	aiUsageLast.prompt = promptTok
	aiUsageLast.completion = completionTok
	aiUsageLast.set = true
	aiUsageMu.Unlock()
}

func takeCapturedAIUsage() (promptTok, completionTok int, ok bool) {
	return takeCapturedAIUsageCtx(context.Background())
}

func takeCapturedAIUsageCtx(ctx context.Context) (promptTok, completionTok int, ok bool) {
	if ctx != nil {
		if id, ok2 := ctx.Value(aiUsageCtxKey{}).(uint64); ok2 {
			if v, loaded := aiUsageSlots.Load(id); loaded {
				slot := v.(*aiUsageSlot)
				if !slot.set {
					return 0, 0, false
				}
				promptTok, completionTok = slot.prompt, slot.completion
				slot.set = false
				slot.prompt, slot.completion = 0, 0
				return promptTok, completionTok, true
			}
		}
	}
	aiUsageMu.Lock()
	defer aiUsageMu.Unlock()
	if !aiUsageLast.set {
		return 0, 0, false
	}
	promptTok, completionTok = aiUsageLast.prompt, aiUsageLast.completion
	aiUsageLast.set = false
	aiUsageLast.prompt, aiUsageLast.completion = 0, 0
	return promptTok, completionTok, true
}

// captureAIUsageFromJSON extracts usage from a chat/completions JSON object or SSE data payload.
func captureAIUsageFromJSON(raw []byte) {
	captureAIUsageFromJSONCtx(context.Background(), raw)
}

func captureAIUsageFromJSONCtx(ctx context.Context, raw []byte) {
	var v map[string]any
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	p, c := tokenUsage(v)
	captureAIUsageCtx(ctx, p, c)
}

type aiModelAgg struct {
	Count  int     `json:"count"`
	Fail   int     `json:"fail"`
	Tokens int64   `json:"tokens"`
	Cost   float64 `json:"cost"`
	AvgMs  int64   `json:"avg_ms"`
}

func (p *pgStore) aiCallByModelFromPG(sinceTs int64) map[string]aiModelAgg {
	out := map[string]aiModelAgg{}
	if p == nil || p.db == nil {
		return out
	}
	rows, err := p.db.Query(`
SELECT COALESCE(NULLIF(model,''),'(unknown)'),
       COUNT(*),
       COALESCE(SUM(CASE WHEN NOT ok THEN 1 ELSE 0 END),0),
       COALESCE(SUM(GREATEST(approx_tokens, prompt_tokens+completion_tokens)),0),
       COALESCE(SUM(cost_estimate),0),
       COALESCE(SUM(latency_ms),0)
FROM ai_call_events_p WHERE ts >= $1
GROUP BY 1 ORDER BY COUNT(*) DESC LIMIT 40`, sinceTs)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var model string
		var cnt, fl int
		var tokens, latencySum int64
		var costF float64
		if rows.Scan(&model, &cnt, &fl, &tokens, &costF, &latencySum) != nil {
			continue
		}
		avg := int64(0)
		if cnt > 0 {
			avg = latencySum / int64(cnt)
		}
		out[model] = aiModelAgg{Count: cnt, Fail: fl, Tokens: tokens, Cost: costF, AvgMs: avg}
	}
	return out
}

type aiTaskCostAgg struct {
	Count int     `json:"count"`
	Fail  int     `json:"fail"`
	Cost  float64 `json:"cost"`
	Tokens int64  `json:"tokens"`
	AvgMs int64   `json:"avg_ms"`
}

func (p *pgStore) aiCallByTaskCostFromPG(sinceTs int64) map[string]aiTaskCostAgg {
	out := map[string]aiTaskCostAgg{}
	if p == nil || p.db == nil {
		return out
	}
	rows, err := p.db.Query(`
SELECT COALESCE(NULLIF(task,''),'(unknown)'),
       COUNT(*),
       COALESCE(SUM(CASE WHEN NOT ok THEN 1 ELSE 0 END),0),
       COALESCE(SUM(cost_estimate),0),
       COALESCE(SUM(GREATEST(approx_tokens, prompt_tokens+completion_tokens)),0),
       COALESCE(SUM(latency_ms),0)
FROM ai_call_events_p WHERE ts >= $1
GROUP BY 1 ORDER BY SUM(cost_estimate) DESC NULLS LAST LIMIT 40`, sinceTs)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var task string
		var cnt, fl int
		var costF float64
		var tokens, latSum int64
		if rows.Scan(&task, &cnt, &fl, &costF, &tokens, &latSum) != nil {
			continue
		}
		avg := int64(0)
		if cnt > 0 {
			avg = latSum / int64(cnt)
		}
		out[task] = aiTaskCostAgg{Count: cnt, Fail: fl, Cost: costF, Tokens: tokens, AvgMs: avg}
	}
	return out
}
