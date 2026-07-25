package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aiops-monitor/shared"
)

func (s *Server) handleAgentContainers(w http.ResponseWriter, r *http.Request) {
	var rep shared.ContainerReport
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if rep.HostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_id required"})
		return
	}
	fp := r.Header.Get("X-Agent-Fingerprint")
	if fp == "" {
		fp = r.URL.Query().Get("fp")
	}
	if !s.forwardFingerprintOKByHost(rep.HostID, fp) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "fingerprint mismatch"})
		return
	}
	hostname := rep.HostName
	if h := s.hostByID(rep.HostID); h != nil && h.Hostname != "" {
		hostname = h.Hostname
	}
	if rep.Error != "" {
		slog.Warn("容器采集失败，保留上一份清单", "host_id", rep.HostID, "err", rep.Error)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if s.pg != nil {
		s.pg.upsertContainerInventory(rep.HostID, hostname, rep.Runtime, rep.Containers)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleContainerList(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"inventories": []any{}})
		return
	}
	host := r.URL.Query().Get("host")
	var rows []map[string]any
	var err error
	if host != "" {
		if inv, ok := s.pg.getContainerInventory(host); ok {
			rows = []map[string]any{inv}
		}
	} else {
		rows, err = s.pg.getAllContainerInventories()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"inventories": rows, "ts": time.Now().Unix()})
}

func (s *Server) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	hostID := strings.TrimSpace(r.PathValue("hostID"))
	cid := strings.TrimSpace(r.PathValue("id"))
	var req struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "start", "stop", "restart":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be start|stop|restart"})
		return
	}
	args := map[string]string{"action": action, "id": cid, "name": strings.TrimSpace(req.Name)}
	out, err := s.runAgentModule(hostID, "container_action", args, 90)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "output": out})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("容器 %s：host=%s id=%s", action, hostID, cid)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out})
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	hostID := strings.TrimSpace(r.PathValue("hostID"))
	cid := strings.TrimSpace(r.PathValue("id"))
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "200"
	}
	if _, err := strconv.Atoi(tail); err != nil {
		tail = "200"
	}
	out, err := s.runAgentModule(hostID, "container_logs", map[string]string{"id": cid, "tail": tail}, 60)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "log": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"log": out})
}
