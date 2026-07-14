package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// maskDataSource returns a copy with the auth password masked for browser display.
func maskDataSource(d DataSource) DataSource {
	d.AuthPass = maskSecret(d.AuthPass)
	return d
}

// GET /api/v1/datasources — list all configured data sources (passwords masked).
func (s *Server) handleDataSourceList(w http.ResponseWriter, r *http.Request) {
	list := s.cfg.ListDataSources()
	out := make([]DataSource, len(list))
	for i, d := range list {
		out[i] = maskDataSource(d)
	}
	writeJSON(w, http.StatusOK, out)
}

// validateDataSource checks the required fields + supported type.
func validateDataSource(ds *DataSource) string {
	ds.Name = strings.TrimSpace(ds.Name)
	ds.URL = strings.TrimSpace(ds.URL)
	ds.Type = strings.TrimSpace(ds.Type)
	if ds.Name == "" || ds.URL == "" {
		return "名称和地址必填"
	}
	if !strings.HasPrefix(ds.URL, "http://") && !strings.HasPrefix(ds.URL, "https://") {
		return "地址需以 http:// 或 https:// 开头"
	}
	if ds.Type != "loki" && ds.Type != "prometheus" {
		return "类型仅支持 loki / prometheus"
	}
	return ""
}

// POST /api/v1/datasources — create a data source.
func (s *Server) handleDataSourceCreate(w http.ResponseWriter, r *http.Request) {
	var ds DataSource
	if err := json.NewDecoder(r.Body).Decode(&ds); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if msg := validateDataSource(&ds); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	saved, err := s.cfg.AddDataSource(ds)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	actor := s.clientIP(r)
	if u, ok := s.currentUser(r); ok && u.Username != "" {
		actor = u.Username
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: actor, Message: "接入数据源 " + saved.Type + "：" + saved.Name})
	writeJSON(w, http.StatusOK, maskDataSource(saved))
}

// PUT /api/v1/datasources/{id} — update a data source.
func (s *Server) handleDataSourceUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var ds DataSource
	if err := json.NewDecoder(r.Body).Decode(&ds); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if msg := validateDataSource(&ds); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	if err := s.cfg.UpdateDataSource(id, ds); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/v1/datasources/{id} — delete a data source.
func (s *Server) handleDataSourceDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.cfg.DeleteDataSource(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/v1/datasources/test — test connectivity of the POSTed config. When the
// body carries an id with a blank/masked password, the stored password is used, so
// editing an existing source can be re-tested without re-typing the secret.
func (s *Server) handleDataSourceTest(w http.ResponseWriter, r *http.Request) {
	var ds DataSource
	if err := json.NewDecoder(r.Body).Decode(&ds); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if ds.ID != "" && (ds.AuthPass == "" || strings.Contains(ds.AuthPass, "****")) {
		if saved, ok := s.cfg.GetDataSource(ds.ID); ok {
			ds.AuthPass = saved.AuthPass
		}
	}
	if msg := validateDataSource(&ds); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	if err := testDataSource(ds); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/v1/datasources/{id}/query — run a query against a saved data source.
// Body: {query, limit, since_min}. Used by the query UI and (indirectly) the AI.
func (s *Server) handleDataSourceQuery(w http.ResponseWriter, r *http.Request) {
	ds, ok := s.cfg.GetDataSource(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "数据源不存在"})
		return
	}
	var req struct {
		Query    string `json:"query"`
		Limit    int    `json:"limit"`
		SinceMin int    `json:"since_min"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.Query) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "查询语句必填"})
		return
	}
	result, err := queryDataSource(ds, req.Query, req.Limit, req.SinceMin)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}
