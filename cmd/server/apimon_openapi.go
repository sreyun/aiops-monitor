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

var openAPIAllMethods = []string{"get", "post", "put", "delete", "patch", "head"}

// normalizeOpenAPIMethods builds the allow-set. Empty / "all" / "*" → all standard methods.
func normalizeOpenAPIMethods(in []string) map[string]bool {
	all := make(map[string]bool, len(openAPIAllMethods))
	for _, m := range openAPIAllMethods {
		all[m] = true
	}
	if len(in) == 0 {
		return all
	}
	out := map[string]bool{}
	for _, raw := range in {
		m := strings.ToLower(strings.TrimSpace(raw))
		if m == "" || m == "all" || m == "*" {
			return all
		}
		if all[m] {
			out[m] = true
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

// parseOpenAPI 从 OpenAPI/Swagger JSON 解析出接口清单；baseURL 非空则覆盖规范内推断的基址。
// allowMethods 为空表示全部标准方法；否则仅导入所列方法（如 ["get"]）。
func parseOpenAPI(spec []byte, baseURL string, allowMethods ...string) ([]APIEndpoint, error) {
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
	methods := normalizeOpenAPIMethods(allowMethods)
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

func sanitizeOpenAPICommonHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// handleImportOpenAPI 从 OpenAPI/Swagger 规范批量导入接口，落为一个业务系统并立即探测一次。
// 支持直接粘贴 JSON，或传 spec_url（doc.html / Knife4j / api-docs）由服务端自动拉取。
func (s *Server) handleImportOpenAPI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SystemName    string            `json:"system_name"`
		BaseURL       string            `json:"base_url"`
		Spec          string            `json:"spec"`
		SpecURL       string            `json:"spec_url"`
		Group         string            `json:"group"`
		Methods       []string          `json:"methods"` // empty / ["all"] = 全部；默认前端传 ["get"]
		CommonHeaders map[string]string `json:"common_headers"`
		CommonBody    string            `json:"common_body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	req.SystemName = strings.TrimSpace(req.SystemName)
	req.Spec = strings.TrimSpace(req.Spec)
	req.SpecURL = strings.TrimSpace(req.SpecURL)
	req.Group = strings.TrimSpace(req.Group)
	req.BaseURL = strings.TrimSpace(req.BaseURL)

	if req.Spec == "" && req.SpecURL != "" {
		fetched, err := fetchOpenAPIFromDocURL(req.SpecURL, req.Group, req.BaseURL)
		if err != nil {
			body := map[string]any{"error": "拉取 OpenAPI 失败：" + err.Error()}
			if fetched != nil && len(fetched.Groups) > 0 {
				body["groups"] = fetched.Groups
			}
			writeJSON(w, http.StatusBadRequest, body)
			return
		}
		req.Spec = fetched.Spec
		if req.BaseURL == "" {
			req.BaseURL = fetched.SuggestedBase
		}
		if req.SystemName == "" {
			req.SystemName = fetched.SuggestedName
		}
	}
	if req.SystemName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "业务系统名称不能为空"})
		return
	}
	if req.Spec == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请粘贴 OpenAPI JSON，或填写可访问的文档地址（如 Knife4j doc.html）后拉取"})
		return
	}
	eps, err := parseOpenAPI([]byte(req.Spec), req.BaseURL, req.Methods...)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "OpenAPI 解析失败：" + err.Error()})
		return
	}
	if len(eps) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "未从规范解析出任何接口（请检查 paths、HTTP 方法筛选与基址）"})
		return
	}
	sys := APISystem{
		Name:          req.SystemName,
		IntervalSec:   60,
		Level:         "critical",
		Enabled:       true,
		Endpoints:     eps,
		CommonHeaders: sanitizeOpenAPICommonHeaders(req.CommonHeaders),
		CommonBody:    strings.TrimSpace(req.CommonBody),
	}
	saved, err := s.cfg.UpsertAPISystem(sys)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.apimon.runNow(saved.ID)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: "OpenAPI 导入业务系统：" + saved.Name + "（" + strconv.Itoa(len(eps)) + " 接口）"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID, "count": len(eps)})
}

// handleFetchOpenAPI discovers/downloads OpenAPI JSON from a Knife4j/doc.html/api-docs URL.
func (s *Server) handleFetchOpenAPI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL     string `json:"url"`
		Group   string `json:"group"`
		BaseURL string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请填写文档地址"})
		return
	}
	res, err := fetchOpenAPIFromDocURL(req.URL, req.Group, req.BaseURL)
	if err != nil {
		body := map[string]any{"error": err.Error()}
		if res != nil {
			if len(res.Groups) > 0 {
				body["groups"] = res.Groups
			}
			if len(res.Notes) > 0 {
				body["notes"] = res.Notes
			}
		}
		writeJSON(w, http.StatusBadGateway, body)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
