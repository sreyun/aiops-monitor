package main

import (
	"fmt"
	"strings"
)

// memoryWriteOpts carries optional scope / verification flags for RAG writes.
type memoryWriteOpts struct {
	ServiceID string
	Category  string
	Verified  bool // true = human/verify-backed; boosted in retrieval
}

// rememberAIScoped is rememberAI with BusinessService / host category / verified flags.
func (s *Server) rememberAIScoped(kind, source, content string, opts memoryWriteOpts) bool {
	if s.pg == nil || s.memoryCh == nil {
		return false
	}
	content = strings.TrimSpace(content)
	if len([]rune(content)) < 12 {
		return false
	}
	cfg := s.cfg.AIConfig()
	if !embedReady(cfg) {
		return false
	}
	select {
	case s.memoryCh <- memoryJob{
		kind: kind, source: source, content: content,
		serviceID: strings.TrimSpace(opts.ServiceID),
		category:  strings.TrimSpace(opts.Category),
		verified:  opts.Verified,
	}:
		return true
	default:
		return false // queue full; caller may retry later
	}
}

// rememberFromIncident writes a scoped memory using the incident's host/service context.
func (s *Server) rememberFromIncident(inc Incident, kind, content string, verified bool) bool {
	svc, cat := s.memoryScopeFromIncident(inc)
	src := fmt.Sprintf("incident:%d", inc.ID)
	return s.rememberAIScoped(kind, src, content, memoryWriteOpts{
		ServiceID: svc, Category: cat, Verified: verified,
	})
}

func (s *Server) memoryScopeFromIncident(inc Incident) (serviceID, category string) {
	if s == nil {
		return "", ""
	}
	if s.store != nil && strings.TrimSpace(inc.HostID) != "" {
		if h, ok := s.store.GetHost(inc.HostID); ok && h != nil {
			category = h.Category
		}
	}
	if s.cfg != nil && strings.TrimSpace(inc.HostID) != "" {
		for _, svc := range s.cfg.BusinessServices() {
			for _, hid := range svc.HostIDs {
				if hid == inc.HostID {
					return svc.ID, category
				}
			}
		}
	}
	return "", category
}

func (s *Server) memoryScopeFromQuery(query string) (serviceID, category string) {
	if s == nil {
		return "", ""
	}
	return s.skillRetrievalScope(query)
}

// citationIsStrong reports whether a citation is substantive (not just a heartbeat chip).
func citationIsStrong(c RAGCitation) bool {
	k := strings.ToLower(strings.TrimSpace(c.Kind))
	switch k {
	case "metric", "alert", "log", "sql", "knowledge", "resolution", "experience", "pitfall", "weknora":
		return true
	}
	blob := strings.ToLower(c.Source + " " + c.Title + " " + c.Summary)
	for _, needle := range []string{
		"metric", "alert", "log", "sql", "explain", "promql", "logql",
		"cpu", "mem", "disk", "日志", "告警", "指标",
	} {
		if strings.Contains(blob, needle) {
			return true
		}
	}
	return false
}

// diagnosisEvidenceOK requires ≥1 strong citation, or ≥3 mixed citations.
func diagnosisEvidenceOK(cites []RAGCitation) bool {
	if len(cites) == 0 {
		return false
	}
	strong := 0
	for _, c := range cites {
		if citationIsStrong(c) {
			strong++
		}
	}
	return strong >= 1 || len(cites) >= 3
}

// memoryMatchesScope mirrors skillMatchesScope for memory rows.
func memoryMatchesScope(serviceIDs, categories, wantSvc, wantCat string) bool {
	sk := Skill{ServiceIDs: serviceIDs, Categories: categories}
	return skillMatchesScope(sk, wantSvc, wantCat)
}
