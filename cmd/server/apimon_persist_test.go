package main

import (
	"path/filepath"
	"testing"
)

// TestAPISystemPersistenceRoundTrip 端到端验证「保存→落盘→重载」后，本轮新增的所有 APISystem/
// APIEndpoint 字段都真实持久化（CommonBody / HostIDs / Env / 接口 TimeoutSec/Retries/Distributed/
// Protocol / Headers）。用文件后端的 ConfigStore 重新打开同一文件断言，避免只测内存。
func TestAPISystemPersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.json")
	cs, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	sys := APISystem{
		Name: "订单系统", IntervalSec: 60, Level: "critical", Enabled: true,
		Env: "prod", CommonBody: `{"appId":"1"}`, HostIDs: []string{"h1", "h2"}, MaintUntil: 9999999999,
		CommonHeaders: map[string]string{"Authorization": "Bearer x"},
		Endpoints: []APIEndpoint{{
			Name: "下单", URL: "https://x/order", Method: "POST", Enabled: true,
			TimeoutSec: 20, Retries: 2, Distributed: true, Protocol: "graphql",
			Headers: map[string]string{"X-K": "v"},
		}},
	}
	saved, err := cs.UpsertAPISystem(sys)
	if err != nil {
		t.Fatal(err)
	}

	// 重新从同一文件加载，验证所有字段落盘
	cs2, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := cs2.APISystems()
	if len(got) != 1 {
		t.Fatalf("应有 1 个系统, 得 %d", len(got))
	}
	g := got[0]
	if g.ID != saved.ID || g.Env != "prod" || g.CommonBody != `{"appId":"1"}` || g.MaintUntil != 9999999999 {
		t.Fatalf("系统级新字段未落盘: %+v", g)
	}
	if len(g.HostIDs) != 2 || g.HostIDs[0] != "h1" || g.HostIDs[1] != "h2" {
		t.Fatalf("HostIDs 未落盘: %v", g.HostIDs)
	}
	if g.CommonHeaders["Authorization"] != "Bearer x" {
		t.Fatalf("CommonHeaders 未落盘(或加解密不对称): %v", g.CommonHeaders)
	}
	if len(g.Endpoints) != 1 {
		t.Fatalf("接口未落盘")
	}
	e := g.Endpoints[0]
	if e.TimeoutSec != 20 || e.Retries != 2 || !e.Distributed || e.Protocol != "graphql" {
		t.Fatalf("接口新字段未落盘: %+v", e)
	}
	if e.Headers["X-K"] != "v" {
		t.Fatalf("接口 Headers 未落盘: %v", e.Headers)
	}
}

// TestAPITransactionPersistenceRoundTrip 验证合成事务（Vars / Steps / Extract / Headers）落盘。
func TestAPITransactionPersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.json")
	cs, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	txn := APITransaction{
		Name: "下单流程", IntervalSec: 60, Level: "critical", Enabled: true,
		Vars: map[string]string{"base": "https://x"},
		Steps: []APIStep{{
			Name: "登录", Method: "POST", URL: "{{base}}/login", TimeoutSec: 15,
			Extract: map[string]string{"token": "data.token"},
			Headers: map[string]string{"X": "y"},
		}},
	}
	saved, err := cs.UpsertAPITransaction(txn)
	if err != nil {
		t.Fatal(err)
	}
	cs2, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := cs2.APITransactions()
	if len(got) != 1 {
		t.Fatalf("应有 1 个事务, 得 %d", len(got))
	}
	g := got[0]
	if g.ID != saved.ID || g.Vars["base"] != "https://x" {
		t.Fatalf("事务字段未落盘: %+v", g)
	}
	if len(g.Steps) != 1 || g.Steps[0].Extract["token"] != "data.token" || g.Steps[0].Headers["X"] != "y" {
		t.Fatalf("步骤/提取变量未落盘: %+v", g.Steps)
	}
}

// TestSLOAPISourcePersistenceRoundTrip 验证 SLO 的 api 源(APIID)落盘。
func TestSLOAPISourcePersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.json")
	cs, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := cs.UpsertSLO(SLO{Name: "订单可用性", SourceType: "api", APIID: "ep1", Target: 99.9, WindowDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	cs2, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := cs2.SLOs()
	if len(got) != 1 || got[0].ID != saved.ID || got[0].SourceType != "api" || got[0].APIID != "ep1" {
		t.Fatalf("SLO api 源未落盘: %+v", got)
	}
}
