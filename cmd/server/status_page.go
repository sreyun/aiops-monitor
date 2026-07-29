package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// StatusPageConfig controls the public status page.
type StatusPageConfig struct {
	Enabled     bool   `json:"enabled"`
	Title       string `json:"title,omitempty"`
	Subtitle    string `json:"subtitle,omitempty"`
	PublicToken string `json:"public_token,omitempty"` // optional bearer for JSON API
}

func (cs *ConfigStore) StatusPage() StatusPageConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	c := cs.cfg.StatusPage
	if c.Title == "" {
		c.Title = "服务状态"
	}
	return c
}

func (cs *ConfigStore) SetStatusPage(c StatusPageConfig) error {
	cs.mu.Lock()
	cs.cfg.StatusPage = c
	cs.mu.Unlock()
	return cs.save()
}

type statusComponent struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // operational|degraded|major_outage|maintenance
	Detail string `json:"detail,omitempty"`
}

type statusIncident struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Severity  string `json:"severity,omitempty"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updated_at"`
}

type statusPagePayload struct {
	Title       string            `json:"title"`
	Subtitle    string            `json:"subtitle,omitempty"`
	Overall     string            `json:"overall"`
	UpdatedAt   int64             `json:"updated_at"`
	Components  []statusComponent `json:"components"`
	Incidents   []statusIncident  `json:"incidents"`
	SLOBreaches int               `json:"slo_breaches"`
}

func (s *Server) buildStatusPagePayload() statusPagePayload {
	cfg := s.cfg.StatusPage()
	out := statusPagePayload{
		Title: cfg.Title, Subtitle: cfg.Subtitle,
		UpdatedAt: time.Now().Unix(), Overall: "operational",
		Components: []statusComponent{}, Incidents: []statusIncident{},
	}
	// Active critical/warning alerts → components
	crit, warn := 0, 0
	if s.notifier != nil {
		for _, a := range s.notifier.ActiveAlerts() {
			switch strings.ToLower(a.Level) {
			case "critical", "crit", "fatal":
				crit++
			case "warning", "warn":
				warn++
			}
		}
	}
	hostStatus := "operational"
	hostDetail := "主机监控正常"
	if crit > 0 {
		hostStatus = "major_outage"
		hostDetail = fmt.Sprintf("%d 条严重告警", crit)
	} else if warn > 0 {
		hostStatus = "degraded"
		hostDetail = fmt.Sprintf("%d 条警告告警", warn)
	}
	out.Components = append(out.Components, statusComponent{ID: "hosts", Name: "主机与指标", Status: hostStatus, Detail: hostDetail})

	// Open incidents
	if s.incidents != nil {
		for _, inc := range s.incidents.List() {
			st := strings.ToLower(inc.Status)
			if st == "resolved" || st == "closed" {
				continue
			}
			upd := inc.CreatedAt
			if inc.AckedAt > upd {
				upd = inc.AckedAt
			}
			out.Incidents = append(out.Incidents, statusIncident{
				ID: inc.ID, Title: inc.Title, Severity: inc.Severity, Status: inc.Status, UpdatedAt: upd,
			})
			if len(out.Incidents) >= 20 {
				break
			}
		}
	}
	if len(out.Incidents) > 0 && out.Overall == "operational" {
		out.Overall = "degraded"
	}
	if crit > 0 {
		out.Overall = "major_outage"
	}

	// SLO breaches
	if s.slos != nil {
		for _, st := range s.slos.Evaluate() {
			if st.Breaching {
				out.SLOBreaches++
			}
		}
	}
	if out.SLOBreaches > 0 {
		out.Components = append(out.Components, statusComponent{
			ID: "slo", Name: "SLO 目标", Status: "degraded",
			Detail: fmt.Sprintf("%d 个 SLO 正在违约", out.SLOBreaches),
		})
		if out.Overall == "operational" {
			out.Overall = "degraded"
		}
	} else {
		out.Components = append(out.Components, statusComponent{ID: "slo", Name: "SLO 目标", Status: "operational", Detail: "全部达标"})
	}
	return out
}

func (s *Server) handlePublicStatusJSON(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.StatusPage()
	if !cfg.Enabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "status page disabled"})
		return
	}
	if tok := strings.TrimSpace(cfg.PublicToken); tok != "" {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" {
			got = r.URL.Query().Get("token")
		}
		if got != tok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
	}
	writeJSON(w, http.StatusOK, s.buildStatusPagePayload())
}

func (s *Server) handlePublicStatusHTML(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.StatusPage()
	if !cfg.Enabled {
		http.NotFound(w, r)
		return
	}
	p := s.buildStatusPagePayload()
	color := map[string]string{
		"operational": "#16a34a", "degraded": "#ca8a04", "major_outage": "#dc2626", "maintenance": "#2563eb",
	}
	oc := color[p.Overall]
	if oc == "" {
		oc = "#64748b"
	}
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<title>` + htmlEsc(p.Title) + `</title>`)
	b.WriteString(`<style>body{font-family:system-ui,sans-serif;max-width:720px;margin:40px auto;padding:0 16px;color:#0f172a;background:#f8fafc}`)
	b.WriteString(`.badge{display:inline-block;padding:4px 10px;border-radius:999px;color:#fff;font-size:13px;background:` + oc + `}`)
	b.WriteString(`.card{background:#fff;border:1px solid #e2e8f0;border-radius:12px;padding:16px;margin:12px 0}`)
	b.WriteString(`.row{display:flex;justify-content:space-between;gap:12px;padding:8px 0;border-bottom:1px solid #f1f5f9}`)
	b.WriteString(`h1{font-size:28px;margin:0 0 8px} .muted{color:#64748b;font-size:13px}</style></head><body>`)
	b.WriteString(`<h1>` + htmlEsc(p.Title) + `</h1>`)
	if p.Subtitle != "" {
		b.WriteString(`<p class="muted">` + htmlEsc(p.Subtitle) + `</p>`)
	}
	b.WriteString(`<p><span class="badge">` + htmlEsc(p.Overall) + `</span></p>`)
	b.WriteString(`<div class="card"><h3>组件</h3>`)
	for _, c := range p.Components {
		b.WriteString(`<div class="row"><span>` + htmlEsc(c.Name) + `</span><span class="muted">` + htmlEsc(c.Status) + ` · ` + htmlEsc(c.Detail) + `</span></div>`)
	}
	b.WriteString(`</div><div class="card"><h3>进行中事件</h3>`)
	if len(p.Incidents) == 0 {
		b.WriteString(`<p class="muted">当前无进行中事件</p>`)
	} else {
		for _, inc := range p.Incidents {
			b.WriteString(`<div class="row"><span>` + htmlEsc(inc.Title) + `</span><span class="muted">` + htmlEsc(inc.Status) + `</span></div>`)
		}
	}
	b.WriteString(`</div><p class="muted">更新于 ` + time.Unix(p.UpdatedAt, 0).Local().Format("2006-01-02 15:04:05") + `</p>`)
	b.WriteString(`</body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(b.String()))
}

func htmlEsc(s string) string {
	b, _ := json.Marshal(s)
	if len(b) >= 2 {
		return string(b[1 : len(b)-1])
	}
	return s
}

func (s *Server) handleGetStatusPageConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.StatusPage())
}

func (s *Server) handleSetStatusPageConfig(w http.ResponseWriter, r *http.Request) {
	var c StatusPageConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.cfg.SetStatusPage(c); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
