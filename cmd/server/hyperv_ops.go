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
		Name            string `json:"name"`
		ProcessorCount  int    `json:"processor_count"`
		MemoryMB        int64  `json:"memory_mb"`
		MemoryMinMB     int64  `json:"memory_min_mb"`
		MemoryMaxMB     int64  `json:"memory_max_mb"`
		DynamicMemory   *bool  `json:"dynamic_memory"`
		DynamicMemoryOk bool   `json:"-"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	args := map[string]string{"vm_id": vmID, "name": strings.TrimSpace(req.Name)}
	if req.ProcessorCount > 0 {
		if req.ProcessorCount > 256 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "processor_count out of range"})
			return
		}
		args["processor_count"] = strconv.Itoa(req.ProcessorCount)
	}
	setMem := func(key string, mb int64) bool {
		if mb <= 0 {
			return false
		}
		if mb < 32 || mb > 1024*1024 {
			return false
		}
		args[key] = strconv.FormatInt(mb, 10)
		return true
	}
	if req.MemoryMB > 0 && !setMem("memory_mb", req.MemoryMB) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_mb out of range (32~1048576)"})
		return
	}
	if req.MemoryMinMB > 0 && !setMem("memory_min_mb", req.MemoryMinMB) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_min_mb out of range"})
		return
	}
	if req.MemoryMaxMB > 0 && !setMem("memory_max_mb", req.MemoryMaxMB) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_max_mb out of range"})
		return
	}
	if req.MemoryMinMB > 0 && req.MemoryMaxMB > 0 && req.MemoryMinMB > req.MemoryMaxMB {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_min_mb cannot exceed memory_max_mb"})
		return
	}
	if req.MemoryMB > 0 && req.MemoryMinMB > 0 && req.MemoryMB < req.MemoryMinMB {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_mb cannot be below memory_min_mb"})
		return
	}
	if req.MemoryMB > 0 && req.MemoryMaxMB > 0 && req.MemoryMB > req.MemoryMaxMB {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_mb cannot exceed memory_max_mb"})
		return
	}
	if req.DynamicMemory != nil {
		if *req.DynamicMemory {
			args["dynamic_memory"] = "true"
		} else {
			args["dynamic_memory"] = "false"
		}
	}
	if args["processor_count"] == "" && args["memory_mb"] == "" &&
		args["memory_min_mb"] == "" && args["memory_max_mb"] == "" && args["dynamic_memory"] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no config changes provided"})
		return
	}
	out, err := s.runAgentModule(hostID, "hyperv_set", args, 120)
	if err != nil {
		msg := err.Error()
		if strings.Contains(out, "NEED_VM_OFF") {
			msg = out
		} else if strings.TrimSpace(out) != "" {
			msg = out
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": msg, "output": out})
		return
	}
	dynLabel := "-"
	if req.DynamicMemory != nil {
		if *req.DynamicMemory {
			dynLabel = "on"
		} else {
			dynLabel = "off"
		}
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("Hyper-V 改配：host=%s vm=%s cpu=%d mem=%d min=%d max=%d dyn=%s",
			hostID, vmID, req.ProcessorCount, req.MemoryMB, req.MemoryMinMB, req.MemoryMaxMB, dynLabel)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out})
}
