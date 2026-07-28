package main

import (
	"encoding/json"
	"sync"
)

// Package-level last-usage capture so stream/non-stream chat paths can stash
// provider token counts without changing every call signature. recordAICall*
// consumes and clears the value.
var (
	aiUsageMu   sync.Mutex
	aiUsageLast struct {
		prompt, completion int
		set                bool
	}
)

func captureAIUsage(promptTok, completionTok int) {
	if promptTok <= 0 && completionTok <= 0 {
		return
	}
	aiUsageMu.Lock()
	aiUsageLast.prompt = promptTok
	aiUsageLast.completion = completionTok
	aiUsageLast.set = true
	aiUsageMu.Unlock()
}

func takeCapturedAIUsage() (promptTok, completionTok int, ok bool) {
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
	var v map[string]any
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	p, c := tokenUsage(v)
	captureAIUsage(p, c)
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
FROM ai_call_events WHERE ts >= $1
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
