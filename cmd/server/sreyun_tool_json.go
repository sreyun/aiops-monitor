package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// toolJSONSoftLimit is a soft ceiling for MCP / Hermes tool payloads.
// Never mid-cut JSON below this — shrink structured fields instead.
const toolJSONSoftLimit = 48 << 10 // 48 KiB

func toolResultJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"json marshal failed"}`
	}
	return string(b)
}

// toolResultJSONBounded marshals v. If over maxBytes, wraps a compact notice
// rather than slicing bytes mid-JSON (which breaks Agent parsers).
func toolResultJSONBounded(v any, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = toolJSONSoftLimit
	}
	b, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"json marshal failed"}`
	}
	if len(b) <= maxBytes {
		return string(b)
	}
	notice := map[string]any{
		"ok":            false,
		"truncated":     true,
		"bytes":         len(b),
		"limit_bytes":   maxBytes,
		"hint":          "结果过大：请缩小范围（指定 host_id / limit / offset），或先用 list_hosts / query_containers 摘要模式。",
		"preview_error": "payload_too_large",
	}
	out, _ := json.Marshal(notice)
	return string(out)
}

func argString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func argInt(args map[string]any, key string, def int) int {
	if args == nil {
		return def
	}
	switch v := args[key].(type) {
	case float64:
		if int(v) > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	case string:
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	case json.Number:
		if n, err := v.Int64(); err == nil && n > 0 {
			return int(n)
		}
	}
	return def
}

func argBool(args map[string]any, key string, def bool) bool {
	if args == nil {
		return def
	}
	switch v := args[key].(type) {
	case bool:
		return v
	case string:
		switch v {
		case "1", "true", "TRUE", "yes", "on":
			return true
		case "0", "false", "FALSE", "no", "off":
			return false
		}
	}
	return def
}
