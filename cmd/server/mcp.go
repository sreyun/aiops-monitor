package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// ============================================================================
// MCP Server —— 把本平台的【只读】运维工具暴露为标准 Model Context Protocol，供外部 Agent
// （如 Nous Hermes Agent、Claude Desktop、Cursor 等 MCP 客户端）连接调用。
//
// 这是「不换引擎、用 MCP 桥接对接外部 Agent」的可逆试水通道：复用 Sreyun 引擎已注册的工具
// 执行器，只导出一个只读白名单（排除会执行代码/变更的工具）。传输 = JSON-RPC over HTTP(POST)，
// Bearer Token 鉴权。默认关闭。主干零绑定——随时关掉即完全撤除。
// Wave 2：支持 scoped token（metrics/logs/sql/…）与限流审计。
// ============================================================================

var mcpReadonlyTools = map[string]bool{
	"query_metrics": true, "search_logs": true, "list_alerts": true,
	"search_similar_cases": true, "search_knowledge": true, "list_datasources": true, "query_datasource": true,
	"list_recent_changes": true, "check_host_health": true,
	"query_hardware": true, "query_hardware_events": true, "query_hardware_history": true,
	"query_hardware_changes": true, "query_netflow": true, "query_hyperv": true,
	"query_snmp": true, "query_interface_traffic": true, "query_traps": true,
	"query_netflow_flows": true,
	"query_containers": true, "query_k8s": true, "locate_resource": true,
}

type jsonRPCReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rawOrNull(id), "result": result})
}
func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rawOrNull(id), "error": map[string]any{"code": code, "message": msg}})
}
func rawOrNull(id json.RawMessage) any {
	if len(id) == 0 {
		return nil
	}
	return id
}

func mcpTokenFingerprint(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:8])
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.AIConfig()
	if !cfg.MCPEnabled || (strings.TrimSpace(cfg.MCPToken) == "" && strings.TrimSpace(cfg.MCPScopedTokensJSON) == "") {
		http.Error(w, "MCP server disabled", http.StatusNotFound)
		return
	}
	tok := strings.TrimPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer ")
	ok, scopes, tokName := resolveMCPAuth(cfg, tok)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed (use POST JSON-RPC)", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	limit := cfg.MCPRateLimitPerMin
	if limit <= 0 {
		limit = 60
	}
	if s.aiGov != nil {
		if ok, used, lim := s.aiGov.checkAndIncrMCPRate(mcpTokenFingerprint(tok)+":"+tokName, limit); !ok {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "MCP rate limit exceeded ("+itoa(used)+"/"+itoa(lim)+" per min)", http.StatusTooManyRequests)
			return
		}
	}
	var req jsonRPCReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCError(w, nil, -32700, "parse error")
		return
	}
	switch req.Method {
	case "initialize":
		protocol := "2025-06-18"
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion != "" {
			protocol = p.ProtocolVersion
		}
		writeRPCResult(w, req.ID, map[string]any{
			"protocolVersion": protocol,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "aiops-monitor", "version": appVersion, "token": tokName, "scopes": scopes},
		})
	case "notifications/initialized", "notifications/cancelled":
		w.WriteHeader(http.StatusAccepted)
	case "ping":
		writeRPCResult(w, req.ID, map[string]any{})
	case "tools/list":
		writeRPCResult(w, req.ID, map[string]any{"tools": s.mcpToolList(scopes)})
	case "tools/call":
		s.mcpToolCall(w, req, scopes, tokName)
	default:
		writeRPCError(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) mcpToolList(scopes []string) []map[string]any {
	out := []map[string]any{}
	if s.sreyun == nil {
		return out
	}
	for name, t := range s.sreyun.tools {
		if !mcpToolAllowedByScopes(name, scopes) {
			continue
		}
		schema := t.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{"name": name, "description": t.Description, "inputSchema": schema})
	}
	return out
}

func (s *Server) mcpToolCall(w http.ResponseWriter, req jsonRPCReq, scopes []string, tokName string) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		writeRPCError(w, req.ID, -32602, "invalid params")
		return
	}
	if !mcpToolAllowedByScopes(p.Name, scopes) || s.sreyun == nil {
		writeRPCError(w, req.ID, -32602, "unknown, not-exposed, or out-of-scope tool: "+p.Name)
		return
	}
	tool, ok := s.sreyun.tools[p.Name]
	if !ok {
		writeRPCError(w, req.ID, -32602, "unknown tool: "+p.Name)
		return
	}
	if s.aiGov != nil {
		s.aiGov.recordTool(aiToolAuditEntry{
			Actor: "mcp:" + tokName, Tool: p.Name, Action: "tools/call", Approved: true,
			Detail: "scopes=" + strings.Join(scopes, ","),
		})
	}
	result, err := tool.Execute(p.Arguments)
	if err != nil {
		writeRPCResult(w, req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "工具执行失败：" + err.Error()}},
			"isError": true,
		})
		return
	}
	writeRPCResult(w, req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": result}},
	})
}
