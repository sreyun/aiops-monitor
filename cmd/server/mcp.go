package main

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// ============================================================================
// MCP Server —— 把本平台的【只读】运维工具暴露为标准 Model Context Protocol，供外部 Agent
// （如 Nous Hermes Agent、Claude Desktop、Cursor 等 MCP 客户端）连接调用。
//
// 这是「不换引擎、用 MCP 桥接对接外部 Agent」的可逆试水通道：复用 Hermes 引擎已注册的工具
// 执行器，只导出一个只读白名单（排除会执行代码/变更的工具）。传输 = JSON-RPC over HTTP(POST)，
// Bearer Token 鉴权。默认关闭。主干零绑定——随时关掉即完全撤除。
// ============================================================================

// mcpReadonlyTools 是允许经 MCP 暴露的只读工具白名单（排除 run_python_action 等执行/变更类）。
var mcpReadonlyTools = map[string]bool{
	"query_metrics": true, "search_logs": true, "list_alerts": true,
	"search_similar_cases": true, "list_datasources": true, "query_datasource": true,
	"list_recent_changes": true, "check_host_health": true, "run_diagnostic": true,
	"query_hardware": true, "query_hardware_events": true, "query_hardware_history": true,
	"query_hardware_changes": true, "query_netflow": true, "query_hyperv": true,
}

type jsonRPCReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // 通知无 id
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

// handleMCP 是 MCP over HTTP(JSON-RPC) 入口。/api/v1/mcp（Bearer Token 鉴权）
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.AIConfig()
	if !cfg.MCPEnabled || strings.TrimSpace(cfg.MCPToken) == "" {
		http.Error(w, "MCP server disabled", http.StatusNotFound)
		return
	}
	// Bearer Token 鉴权（常量时间比较，防时序侧信道）
	tok := strings.TrimPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(tok), []byte(cfg.MCPToken)) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed (use POST JSON-RPC)", http.StatusMethodNotAllowed)
		return
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
			protocol = p.ProtocolVersion // 回显客户端协议版本，最大化兼容
		}
		writeRPCResult(w, req.ID, map[string]any{
			"protocolVersion": protocol,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "aiops-monitor", "version": appVersion},
		})
	case "notifications/initialized", "notifications/cancelled":
		w.WriteHeader(http.StatusAccepted) // 通知无响应体
	case "ping":
		writeRPCResult(w, req.ID, map[string]any{})
	case "tools/list":
		writeRPCResult(w, req.ID, map[string]any{"tools": s.mcpToolList()})
	case "tools/call":
		s.mcpToolCall(w, req)
	default:
		writeRPCError(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

// mcpToolList 把 Hermes 只读工具转成 MCP tool 定义（name/description/inputSchema）。
func (s *Server) mcpToolList() []map[string]any {
	out := []map[string]any{}
	if s.hermes == nil {
		return out
	}
	for name, t := range s.hermes.tools {
		if !mcpReadonlyTools[name] {
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

// mcpToolCall 执行一次只读工具调用并返回 MCP content。
func (s *Server) mcpToolCall(w http.ResponseWriter, req jsonRPCReq) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		writeRPCError(w, req.ID, -32602, "invalid params")
		return
	}
	if !mcpReadonlyTools[p.Name] || s.hermes == nil {
		writeRPCError(w, req.ID, -32602, "unknown or not-exposed tool: "+p.Name)
		return
	}
	tool, ok := s.hermes.tools[p.Name]
	if !ok {
		writeRPCError(w, req.ID, -32602, "unknown tool: "+p.Name)
		return
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
