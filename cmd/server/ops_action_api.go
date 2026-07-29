package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"
)

type opsApplyReq struct {
	Plan     *OpsActionPlan `json:"plan"`
	Raw      string         `json:"raw,omitempty"`
	AllowDDL bool           `json:"allow_ddl,omitempty"`
	Confirm  bool           `json:"confirm"`
	Grant    string         `json:"grant,omitempty"`
}

type opsApplyResult struct {
	OK      bool           `json:"ok"`
	Risk    string         `json:"risk,omitempty"`
	Results []opsActionRes `json:"results"`
	Error   string         `json:"error,omitempty"`
	Plan    *OpsActionPlan `json:"plan,omitempty"`
}

type opsActionRes struct {
	Type       string `json:"type"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	ClientSide bool   `json:"client_side,omitempty"`
	SQL        string `json:"sql,omitempty"`
	Output     any    `json:"output,omitempty"`
}

func (s *Server) handleOpsActionsValidate(w http.ResponseWriter, r *http.Request) {
	var req opsApplyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	plan, err := resolveOpsPlan(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	norm, risk, err := ValidateOpsActionPlan(plan)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	grant := s.issueOpsActionGrant(norm, s.actorName(r), risk)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "risk": risk, "plan": norm, "grant": grant,
		"expires_in": 300,
	})
}

func (s *Server) handleOpsActionsApply(w http.ResponseWriter, r *http.Request) {
	var req opsApplyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "confirm required"})
		return
	}
	plan, err := resolveOpsPlan(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	norm, risk, err := ValidateOpsActionPlan(plan)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	if req.Grant != "" && !s.verifyOpsActionGrant(req.Grant, norm, s.actorName(r), risk) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid or expired grant"})
		return
	}
	actor := s.actorName(r)
	out := opsApplyResult{OK: true, Risk: risk, Plan: norm, Results: make([]opsActionRes, 0, len(norm.Actions))}
	for _, a := range norm.Actions {
		res := s.executeOpsAction(r, a, req.AllowDDL)
		out.Results = append(out.Results, res)
		if !res.OK && !res.ClientSide {
			out.OK = false
			out.Error = res.Error
			break
		}
	}
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "warning", Actor: actor, IP: s.clientIP(r),
		Message: fmt.Sprintf("AI ops plan apply risk=%s actions=%d ok=%v", risk, len(norm.Actions), out.OK),
	})
	status := http.StatusOK
	if !out.OK {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, out)
}

func resolveOpsPlan(req opsApplyReq) (*OpsActionPlan, error) {
	if req.Plan != nil {
		return req.Plan, nil
	}
	if strings.TrimSpace(req.Raw) != "" {
		return parseOpsActionPlanJSON(req.Raw)
	}
	return nil, fmt.Errorf("plan or raw required")
}

func (s *Server) issueOpsActionGrant(plan *OpsActionPlan, actor, risk string) string {
	exp := time.Now().Unix() + 300
	body, _ := json.Marshal(plan)
	payload := fmt.Sprintf("%s|%s|%d|%s", actor, risk, exp, hex.EncodeToString(sha256Sum(body)))
	mac := hmac.New(sha256.New, auditChainSecret())
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payload + "|" + sig
}

func (s *Server) verifyOpsActionGrant(grant string, plan *OpsActionPlan, actor, risk string) bool {
	parts := strings.Split(grant, "|")
	if len(parts) != 5 {
		return false
	}
	if parts[0] != actor || parts[1] != risk {
		return false
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	body, _ := json.Marshal(plan)
	wantHash := hex.EncodeToString(sha256Sum(body))
	if parts[3] != wantHash {
		return false
	}
	payload := strings.Join(parts[:4], "|")
	mac := hmac.New(sha256.New, auditChainSecret())
	mac.Write([]byte(payload))
	expect := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expect), []byte(parts[4]))
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

func (s *Server) executeOpsAction(parent *http.Request, a OpsAction, allowDDL bool) opsActionRes {
	res := opsActionRes{Type: a.Type}
	tgt := a.Target
	params := a.Params
	switch a.Type {
	case "sql_apply":
		res.OK = true
		res.ClientSide = true
		res.SQL = strMap(params, "sql")
		return res

	case "hyperv_power":
		host := strMap(tgt, "host_id")
		vm := strMap(tgt, "vm_id")
		if vm == "" {
			vm = strMap(tgt, "id")
		}
		if vm == "" {
			vm = strMap(tgt, "name")
		}
		body, _ := json.Marshal(map[string]any{"action": strMap(params, "action"), "name": strMap(tgt, "name")})
		return s.invokeOpsHandler(a.Type, parent, http.MethodPost,
			fmt.Sprintf("/api/v1/hyperv/%s/guests/%s/power", host, vm),
			body, map[string]string{"hostID": host, "vmID": vm}, s.handleHyperVPower)

	case "hyperv_config":
		host := strMap(tgt, "host_id")
		vm := strMap(tgt, "vm_id")
		if vm == "" {
			vm = strMap(tgt, "id")
		}
		if vm == "" {
			vm = strMap(tgt, "name")
		}
		bodyMap := map[string]any{"name": strMap(tgt, "name")}
		for _, k := range []string{"processor_count", "memory_mb", "memory_min_mb", "memory_max_mb", "dynamic_memory"} {
			if v, ok := params[k]; ok {
				bodyMap[k] = v
			}
		}
		body, _ := json.Marshal(bodyMap)
		return s.invokeOpsHandler(a.Type, parent, http.MethodPost,
			fmt.Sprintf("/api/v1/hyperv/%s/guests/%s/config", host, vm),
			body, map[string]string{"hostID": host, "vmID": vm}, s.handleHyperVConfig)

	case "container_action":
		host := strMap(tgt, "host_id")
		id := strMap(tgt, "id")
		if id == "" {
			id = strMap(tgt, "container_id")
		}
		body, _ := json.Marshal(map[string]any{"action": strMap(params, "action")})
		return s.invokeOpsHandler(a.Type, parent, http.MethodPost,
			fmt.Sprintf("/api/v1/containers/%s/%s/action", host, id),
			body, map[string]string{"hostID": host, "id": id}, s.handleContainerAction)

	case "container_exec":
		host := strMap(tgt, "host_id")
		id := strMap(tgt, "id")
		if id == "" {
			id = strMap(tgt, "container_id")
		}
		body, _ := json.Marshal(map[string]any{"command": strMap(params, "command"), "timeout_sec": params["timeout_sec"]})
		return s.invokeOpsHandler(a.Type, parent, http.MethodPost,
			fmt.Sprintf("/api/v1/containers/%s/%s/exec", host, id),
			body, map[string]string{"hostID": host, "id": id}, s.handleContainerExec)

	case "k8s_scale":
		cid, ns, name := strMap(tgt, "cluster_id"), strMap(tgt, "namespace"), strMap(tgt, "name")
		body, _ := json.Marshal(map[string]any{"replicas": params["replicas"]})
		return s.invokeOpsHandler(a.Type, parent, http.MethodPost,
			fmt.Sprintf("/api/v1/k8s/clusters/%s/deployments/%s/%s/scale", cid, ns, name),
			body, map[string]string{"id": cid, "ns": ns, "name": name}, s.handleK8sScaleDeployment)

	case "k8s_restart":
		cid, ns, name := strMap(tgt, "cluster_id"), strMap(tgt, "namespace"), strMap(tgt, "name")
		return s.invokeOpsHandler(a.Type, parent, http.MethodPost,
			fmt.Sprintf("/api/v1/k8s/clusters/%s/deployments/%s/%s/restart", cid, ns, name),
			[]byte("{}"), map[string]string{"id": cid, "ns": ns, "name": name}, s.handleK8sRestartDeployment)

	case "k8s_undo":
		cid, ns, name := strMap(tgt, "cluster_id"), strMap(tgt, "namespace"), strMap(tgt, "name")
		return s.invokeOpsHandler(a.Type, parent, http.MethodPost,
			fmt.Sprintf("/api/v1/k8s/clusters/%s/deployments/%s/%s/undo", cid, ns, name),
			[]byte("{}"), map[string]string{"id": cid, "ns": ns, "name": name}, s.handleK8sUndoDeployment)

	case "k8s_delete_pod":
		cid, ns, name := strMap(tgt, "cluster_id"), strMap(tgt, "namespace"), strMap(tgt, "name")
		return s.invokeOpsHandler(a.Type, parent, http.MethodDelete,
			fmt.Sprintf("/api/v1/k8s/clusters/%s/pods/%s/%s", cid, ns, name),
			nil, map[string]string{"id": cid, "ns": ns, "name": name}, s.handleK8sDeletePod)

	case "k8s_exec":
		cid, ns, name := strMap(tgt, "cluster_id"), strMap(tgt, "namespace"), strMap(tgt, "name")
		body, _ := json.Marshal(map[string]any{"command": strMap(params, "command"), "timeout_sec": params["timeout_sec"]})
		return s.invokeOpsHandler(a.Type, parent, http.MethodPost,
			fmt.Sprintf("/api/v1/k8s/clusters/%s/pods/%s/%s/exec", cid, ns, name),
			body, map[string]string{"id": cid, "ns": ns, "name": name}, s.handleK8sPodExec)

	case "host_playbook":
		return s.executeHostPlaybookAction(parent, a)

	case "sql_ddl":
		if !allowDDL {
			res.Error = "DDL not confirmed"
			return res
		}
		conn := strMap(tgt, "connection_id")
		body, _ := json.Marshal(map[string]any{
			"sql": strMap(params, "sql"), "allow_exec": true,
			"timeout_sec": params["timeout_sec"], "verify_sql": strMap(params, "verify_sql"),
			"reason": strMap(params, "reason"),
		})
		// Prefer change-request path when handler exists; otherwise exec-ddl.
		return s.invokeOpsHandler(a.Type, parent, http.MethodPost,
			fmt.Sprintf("/api/v1/sql/connections/%s/exec-ddl", conn),
			body, map[string]string{"id": conn}, s.handleMySQLExecDDL)

	default:
		res.Error = "unsupported type"
		return res
	}
}

func (s *Server) executeHostPlaybookAction(parent *http.Request, a OpsAction) opsActionRes {
	hostID := strMap(a.Target, "host_id")
	stepsAny, _ := a.Params["steps"].([]map[string]any)
	if stepsAny == nil {
		if raw, ok := a.Params["steps"].([]any); ok {
			for _, x := range raw {
				if m, ok := x.(map[string]any); ok {
					stepsAny = append(stepsAny, m)
				}
			}
		}
	}
	pb := map[string]any{
		"id":          fmt.Sprintf("ai-remediation-%d", time.Now().UnixNano()),
		"name":        strMap(a.Params, "name"),
		"description": strMap(a.Params, "description"),
		"steps":       stepsAny,
	}
	if pb["name"] == "" {
		pb["name"] = "AI 修复 · " + hostID
	}
	body, _ := json.Marshal(pb)
	created := s.invokeOpsHandler(a.Type, parent, http.MethodPost, "/api/v1/playbooks", body, nil, s.handleUpsertPlaybook)
	if !created.OK {
		return created
	}
	id := ""
	if m, ok := created.Output.(map[string]any); ok {
		id = strMap(m, "id")
		if id == "" {
			id = strMap(m, "playbook_id")
		}
	}
	if id == "" {
		id = fmt.Sprint(pb["id"])
	}
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/playbooks/"+id+"/execute", nil)
	if parent != nil {
		req = req.WithContext(parent.Context())
		req.Header = parent.Header.Clone()
	}
	req.Header.Set("X-AIOps-Risk-Accepted", "true")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.handleExecutePlaybook(rec, req)
	return decodeOpsRecorder(a.Type, rec)
}

type opsHandlerFunc func(http.ResponseWriter, *http.Request)

func (s *Server) invokeOpsHandler(typ string, parent *http.Request, method, path string, body []byte, pathVals map[string]string, h opsHandlerFunc) opsActionRes {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, path, rdr)
	if err != nil {
		return opsActionRes{Type: typ, OK: false, Error: err.Error()}
	}
	if parent != nil {
		req = req.WithContext(parent.Context())
		req.Header = parent.Header.Clone()
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range pathVals {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return decodeOpsRecorder(typ, rec)
}

func decodeOpsRecorder(typ string, rec *httptest.ResponseRecorder) opsActionRes {
	res := opsActionRes{Type: typ}
	var payload any
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	res.Output = payload
	if rec.Code >= 200 && rec.Code < 300 {
		res.OK = true
		return res
	}
	res.OK = false
	if m, ok := payload.(map[string]any); ok {
		if e := strMap(m, "error"); e != "" {
			res.Error = e
			return res
		}
	}
	res.Error = fmt.Sprintf("HTTP %d", rec.Code)
	return res
}
