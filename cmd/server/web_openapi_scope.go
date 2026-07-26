package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// handleImportWebOpenAPIScope parses OpenAPI/Swagger and attaches unique absolute
// URLs onto a web scan target as ScanURLs (Nuclei multi -u).
func (s *Server) handleImportWebOpenAPIScope(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetID string `json:"target_id"`
		BaseURL  string `json:"base_url"`
		Spec     string `json:"spec"`
		Replace  bool   `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	targetID := strings.TrimSpace(req.TargetID)
	if targetID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target_id required"})
		return
	}
	eps, err := parseOpenAPI([]byte(req.Spec), req.BaseURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "OpenAPI 解析失败：" + err.Error()})
		return
	}
	urls := openAPIEndpointURLs(eps, 80)
	if len(urls) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "未解析出可用 URL"})
		return
	}
	cfg := s.cfg.WebSecurity()
	idx := -1
	for i := range cfg.Targets {
		if cfg.Targets[i].ID == targetID {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "target not found"})
		return
	}
	t := cfg.Targets[idx]
	if req.Replace {
		t.ScanURLs = urls
	} else {
		t.ScanURLs = mergeUniqueURLs(t.ScanURLs, urls, 80)
	}
	if strings.TrimSpace(t.BaseURL) == "" && len(t.ScanURLs) > 0 {
		t.BaseURL = t.ScanURLs[0]
	}
	cfg.Targets[idx] = t
	if err := s.cfg.SetWebSecurity(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "web target OpenAPI scope imported: " + targetID,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"target":     maskWebTarget(t),
		"url_count":  len(t.ScanURLs),
		"imported":   len(urls),
		"sample_urls": urls[:min(5, len(urls))],
	})
}

func openAPIEndpointURLs(eps []APIEndpoint, limit int) []string {
	if limit <= 0 {
		limit = 80
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(eps))
	for _, ep := range eps {
		u := strings.TrimSpace(ep.URL)
		if u == "" {
			continue
		}
		parsed, err := url.Parse(u)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		// Drop query/fragment noise; keep path for Nuclei coverage.
		parsed.RawQuery = ""
		parsed.Fragment = ""
		key := strings.ToLower(parsed.String())
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, parsed.String())
		if len(out) >= limit {
			break
		}
	}
	return out
}

func mergeUniqueURLs(existing, add []string, limit int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(existing)+len(add))
	for _, u := range append(append([]string{}, existing...), add...) {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		k := strings.ToLower(u)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, u)
		if len(out) >= limit {
			break
		}
	}
	return out
}
