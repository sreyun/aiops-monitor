package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// AI growth helpers: preferences, forecast bias notes, ops-pattern → skill proposals.
// All persist via existing rememberAI / skills tables — no new dependencies.

type opsPatternHit struct {
	Key   string
	Count int
	Last  int64
	Steps string
}

type aiGrowthHub struct {
	mu       sync.Mutex
	patterns map[string]*opsPatternHit // actor|fingerprint → hit
}

func newAIGrowthHub() *aiGrowthHub {
	return &aiGrowthHub{patterns: make(map[string]*opsPatternHit)}
}

var growthHub = newAIGrowthHub()

// rememberUserPreference stores durable UI/ops preferences (kind=preference).
func (s *Server) rememberUserPreference(actor, key, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	content := fmt.Sprintf("用户偏好[%s] %s=%s", strings.TrimSpace(actor), key, value)
	src := "preference:" + key
	if actor != "" {
		src += ":" + actor
	}
	s.rememberAI("preference", src, content)
}

// loadPreferenceHints returns recent preference memories for prompt injection.
func (s *Server) loadPreferenceHints(actor string, limit int) string {
	if s.pg == nil || limit <= 0 {
		return ""
	}
	q := "用户偏好"
	if actor != "" {
		q += " " + actor
	}
	text, hits, _ := s.retrieveMemoryDetailed("preference", q, limit)
	if hits == 0 || strings.TrimSpace(text) == "" {
		return ""
	}
	return "【用户长期偏好】\n" + trimLine(text, 1200)
}

// recordForecastOutcome stores bias note when predicted vs actual diverge (>thresholdPct).
func (s *Server) recordForecastOutcome(metric string, predicted, actual, thresholdPct float64) {
	if thresholdPct <= 0 {
		thresholdPct = 15
	}
	if predicted == 0 && actual == 0 {
		return
	}
	base := mathAbs(predicted)
	if base < 1e-9 {
		base = mathAbs(actual)
	}
	if base < 1e-9 {
		return
	}
	biasPct := (actual - predicted) / base * 100
	if mathAbs(biasPct) < thresholdPct {
		return
	}
	dir := "偏高"
	if biasPct > 0 {
		dir = "偏低" // prediction was lower than actual
	} else {
		dir = "偏高"
	}
	content := fmt.Sprintf("预测偏差修正：指标 %s 上次预测值 %.2f，实际 %.2f，预测%s约 %.1f%%；后续同类预测请酌情修正。",
		strings.TrimSpace(metric), predicted, actual, dir, mathAbs(biasPct))
	s.rememberAI("forecast_bias", "forecast:"+strings.TrimSpace(metric), content)
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// forecastBiasHints injects recent bias corrections into AI prompts.
func (s *Server) forecastBiasHints(query string, limit int) string {
	if s.pg == nil || limit <= 0 {
		return ""
	}
	text, hits, _ := s.retrieveMemoryDetailed("forecast_bias", query, limit)
	if hits == 0 || strings.TrimSpace(text) == "" {
		return ""
	}
	return "【预测误差自省】\n" + trimLine(text, 800)
}

// trackOpsPattern records a repeated diagnostic/ops fingerprint; returns a skill proposal when count≥3.
func (s *Server) trackOpsPattern(actor, fingerprint, stepsSummary string) (propose bool, name, trigger, steps string) {
	fingerprint = strings.TrimSpace(strings.ToLower(fingerprint))
	if fingerprint == "" {
		return false, "", "", ""
	}
	key := actor + "|" + fingerprint
	growthHub.mu.Lock()
	defer growthHub.mu.Unlock()
	h := growthHub.patterns[key]
	if h == nil {
		h = &opsPatternHit{Key: fingerprint, Steps: stepsSummary}
		growthHub.patterns[key] = h
	}
	h.Count++
	h.Last = time.Now().Unix()
	if stepsSummary != "" {
		h.Steps = stepsSummary
	}
	if h.Count < 3 {
		return false, "", "", ""
	}
	// Reset counter after proposing to avoid spam.
	h.Count = 0
	name = "自动提议：" + trimLine(fingerprint, 40)
	trigger = "当出现与「" + fingerprint + "」相似的重复运维路径时"
	steps = h.Steps
	if steps == "" {
		steps = "1) 复核现场指标与日志\n2) 定位根因\n3) 执行既定处置并验证\n4) 必要时回滚"
	}
	return true, name, trigger, steps
}

// proposeSkillDraft inserts a draft skill after user-facing confirmation path.
func (s *Server) proposeSkillDraft(name, trigger, steps, tags string) (int64, error) {
	if s.pg == nil {
		return 0, fmt.Errorf("PG 不可用")
	}
	name = strings.TrimSpace(name)
	steps = strings.TrimSpace(steps)
	if name == "" || steps == "" {
		return 0, fmt.Errorf("技能名称与步骤必填")
	}
	if tags == "" {
		tags = "auto-proposed"
	}
	cfg := s.cfg.AIConfig()
	emb := embedText(cfg, name+" "+trigger)
	var id int64
	var err error
	if len(emb) > 0 {
		id, err = s.pg.insertSkill(name, trigger, steps, tags, "auto_proposed", emb)
	} else {
		id, err = s.pg.insertSkillNoEmbed(name, trigger, steps, tags, "auto_proposed")
	}
	if err != nil {
		return 0, err
	}
	_ = s.pg.setSkillStatus(id, "draft")
	return id, nil
}
