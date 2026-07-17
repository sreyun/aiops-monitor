package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"aiops-monitor/shared"
)

// 端到端验证"重装认回身份"：Agent 带着新的随机 id 注册，服务端按指纹回一个规范
// id，Agent 必须改用它**并写回状态文件**——否则下次重启又会用回随机 id。

func fakeRegisterServer(t *testing.T, canonical string, hits *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/register" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		*hits++
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		id := req["host_id"]
		if canonical != "" {
			id = canonical // 服务端按指纹认回既有身份
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "host_id": id})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestAgent(t *testing.T, server, hostID, fp, stateFile string) *Agent {
	t.Helper()
	a := NewAgent([]ServerConfig{{Server: server}}, 0, 0, nil, nil, hostID, "")
	a.identity = shared.Report{HostID: hostID, Hostname: "web01", Fingerprint: fp}
	a.stateFile = stateFile
	return a
}

func TestReconcileIdentityAdoptsCanonicalAndPersists(t *testing.T) {
	hits := 0
	srv := fakeRegisterServer(t, "orig-host-id", &hits)
	state := filepath.Join(t.TempDir(), "agent_state.json")

	a := newTestAgent(t, srv.URL, "brand-new-random-id", "fp-A", state)
	a.reconcileIdentity()

	if a.identity.HostID != "orig-host-id" {
		t.Fatalf("HostID = %q, want orig-host-id（未认回既有身份，历史会被劈成两半）", a.identity.HostID)
	}
	// 必须落盘，否则下次重启又变回随机 id
	b, err := os.ReadFile(state)
	if err != nil {
		t.Fatalf("状态文件未写入: %v", err)
	}
	// 必须带 tag：Go 的字段名匹配是大小写不敏感的，但 HostID 与 host_id 之间
	// 隔着下划线，不加 tag 会静默解析成空字符串（这条断言差点因此形同虚设）。
	var s struct {
		HostID string `json:"host_id"`
		FP     string `json:"fp"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("状态文件解析失败: %v", err)
	}
	if s.HostID != "orig-host-id" || s.FP != "fp-A" {
		t.Errorf("状态文件 = %+v, want host_id=orig-host-id fp=fp-A", s)
	}
}

func TestReconcileIdentityKeepsIDWhenServerAgrees(t *testing.T) {
	hits := 0
	srv := fakeRegisterServer(t, "", &hits) // 回显请求里的 id = 认可当前身份
	state := filepath.Join(t.TempDir(), "agent_state.json")

	a := newTestAgent(t, srv.URL, "my-id", "fp-A", state)
	a.reconcileIdentity()

	if a.identity.HostID != "my-id" {
		t.Errorf("HostID = %q, want my-id（服务端认可时不应改动）", a.identity.HostID)
	}
}

// 服务端不可达时监控不能停摆：保持本地 id 继续跑，下次启动再认。
func TestReconcileIdentitySurvivesUnreachableServer(t *testing.T) {
	state := filepath.Join(t.TempDir(), "agent_state.json")
	a := newTestAgent(t, "http://127.0.0.1:1", "local-id", "fp-A", state)
	a.reconcileIdentity()
	if a.identity.HostID != "local-id" {
		t.Errorf("HostID = %q, want local-id（服务端不可达时应保持原样继续跑）", a.identity.HostID)
	}
}

// 没有机器指纹就无从判定，绝不能去认别人的身份。
func TestReconcileIdentitySkippedWithoutFingerprint(t *testing.T) {
	hits := 0
	srv := fakeRegisterServer(t, "someone-elses-id", &hits)
	state := filepath.Join(t.TempDir(), "agent_state.json")

	a := newTestAgent(t, srv.URL, "local-id", "", state)
	a.reconcileIdentity()

	if a.identity.HostID != "local-id" {
		t.Errorf("HostID = %q, want local-id（无指纹时不得认领任何身份）", a.identity.HostID)
	}
	if hits != 0 {
		t.Errorf("无指纹时不应发起注册，实际 %d 次", hits)
	}
}
