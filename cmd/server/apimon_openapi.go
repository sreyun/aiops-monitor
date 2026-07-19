package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// ============================================================================
// OpenAPI / Swagger 一键导入（迭代 E）
//
// 从 OpenAPI 3 / Swagger 2 的 JSON 规范批量解析出接口清单，落为一个业务系统——
// 免去手工逐个录入，是接入新系统时的关键提效点。基址优先用用户填写，其次从规范推断
// （OpenAPI3 servers / Swagger2 schemes+host+basePath）。路径参数（如 /users/{id}）原样
// 保留，用户可在导入后按需微调。
// ============================================================================

// openAPISpec 是 OpenAPI 3 / Swagger 2 的最小子集（只取生成接口清单所需字段）。
type openAPISpec struct {
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"` // OpenAPI 3
	Host     string   `json:"host"`     // Swagger 2
	BasePath string   `json:"basePath"` // Swagger 2
	Schemes  []string `json:"schemes"`  // Swagger 2
	Paths    map[string]map[string]struct {
		OperationID string `json:"operationId"`
		Summary     string `json:"summary"`
	} `json:"paths"`
}

const openAPIMaxEndpoints = 200 // 防止超大规范一次导入过多接口

// parseOpenAPI 从 OpenAPI/Swagger JSON 解析出接口清单；baseURL 非空则覆盖规范内推断的基址。
func parseOpenAPI(spec []byte, baseURL string) ([]APIEndpoint, error) {
	var doc openAPISpec
	if err := json.Unmarshal(spec, &doc); err != nil {
		return nil, err
	}
	base := strings.TrimSpace(baseURL)
	if base == "" {
		if len(doc.Servers) > 0 {
			base = doc.Servers[0].URL
		} else if doc.Host != "" {
			scheme := "https"
			if len(doc.Schemes) > 0 {
				scheme = doc.Schemes[0]
			}
			base = scheme + "://" + doc.Host + doc.BasePath
		}
	}
	base = strings.TrimRight(base, "/")
	methods := map[string]bool{"get": true, "post": true, "put": true, "delete": true, "patch": true, "head": true}
	// 路径与方法排序，保证导入结果稳定
	paths := make([]string, 0, len(doc.Paths))
	for p := range doc.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var eps []APIEndpoint
	for _, p := range paths {
		ops := doc.Paths[p]
		ms := make([]string, 0, len(ops))
		for m := range ops {
			if methods[strings.ToLower(m)] {
				ms = append(ms, m)
			}
		}
		sort.Strings(ms)
		for _, m := range ms {
			op := ops[m]
			name := op.OperationID
			if name == "" {
				name = op.Summary
			}
			if name == "" {
				name = strings.ToUpper(m) + " " + p
			}
			eps = append(eps, APIEndpoint{
				Name: name, URL: base + p, Method: strings.ToUpper(m), Enabled: true, TimeoutSec: 10,
			})
			if len(eps) >= openAPIMaxEndpoints {
				return eps, nil
			}
		}
	}
	return eps, nil
}

// handleImportOpenAPI 从 OpenAPI/Swagger 规范批量导入接口，落为一个业务系统并立即探测一次。
func (s *Server) handleImportOpenAPI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SystemName string `json:"system_name"`
		BaseURL    string `json:"base_url"`
		Spec       string `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	req.SystemName = strings.TrimSpace(req.SystemName)
	if req.SystemName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "业务系统名称不能为空"})
		return
	}
	eps, err := parseOpenAPI([]byte(req.Spec), req.BaseURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "OpenAPI 解析失败：" + err.Error()})
		return
	}
	if len(eps) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "未从规范解析出任何接口（请检查 paths 与基址）"})
		return
	}
	sys := APISystem{Name: req.SystemName, IntervalSec: 60, Level: "critical", Enabled: true, Endpoints: eps}
	saved, err := s.cfg.UpsertAPISystem(sys)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.apimon.runNow(saved.ID)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: "OpenAPI 导入业务系统：" + saved.Name + "（" + strconv.Itoa(len(eps)) + " 接口）"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID, "count": len(eps)})
}
