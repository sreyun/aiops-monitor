package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleHyperVPower(w http.ResponseWriter, r *http.Request) {
	hostID := strings.TrimSpace(r.PathValue("hostID"))
	vmID := strings.TrimSpace(r.PathValue("vmID"))
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
	case "start", "stop", "restart", "force_stop":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be start|stop|restart|force_stop"})
		return
	}
	args := map[string]string{"action": action, "vm_id": vmID, "name": strings.TrimSpace(req.Name)}
	out, err := s.runAgentModule(hostID, "hyperv_power", args, 120)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "output": out})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("Hyper-V %s：host=%s vm=%s/%s", action, hostID, vmID, req.Name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out})
}

func (s *Server) handleHyperVConfig(w http.ResponseWriter, r *http.Request) {
	hostID := strings.TrimSpace(r.PathValue("hostID"))
	vmID := strings.TrimSpace(r.PathValue("vmID"))
	var req struct {
		Name           string `json:"name"`
		ProcessorCount int    `json:"processor_count"`
		MemoryMB       int64  `json:"memory_mb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	args := map[string]string{"vm_id": vmID, "name": strings.TrimSpace(req.Name)}
	if req.ProcessorCount > 0 {
		args["processor_count"] = strconv.Itoa(req.ProcessorCount)
	}
	if req.MemoryMB > 0 {
		args["memory_mb"] = strconv.FormatInt(req.MemoryMB, 10)
	}
	if args["processor_count"] == "" && args["memory_mb"] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "processor_count or memory_mb required"})
		return
	}
	out, err := s.runAgentModule(hostID, "hyperv_set", args, 120)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "output": out})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("Hyper-V 改配：host=%s vm=%s cpu=%d mem_mb=%d", hostID, vmID, req.ProcessorCount, req.MemoryMB)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out})
}
