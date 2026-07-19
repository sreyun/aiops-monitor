package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSubstVars 验证 {{var}} 替换：已知变量替换、未知变量保留原样。
func TestSubstVars(t *testing.T) {
	vars := map[string]string{"base": "http://x", "token": "abc"}
	if got := substVars("{{base}}/api?t={{ token }}", vars); got != "http://x/api?t=abc" {
		t.Fatalf("substVars 错误: %s", got)
	}
	if got := substVars("{{unknown}}", vars); got != "{{unknown}}" {
		t.Fatalf("未知变量应保留原样: %s", got)
	}
}

// TestTxnExecutor 端到端验证合成事务：步骤1登录拿 token → 提取 → 步骤2带 {{token}} 鉴权下单。
func TestTxnExecutor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			w.Write([]byte(`{"token":"abc123"}`))
		case "/order":
			if r.Header.Get("Authorization") != "Bearer abc123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"code":0}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := newTestConfigStore(t)
	cr := newCheckRunner(cfg, NewStore(), nil, "")
	ar := newAPIRunner(cr, cfg, NewStore(), nil, newVMWriter(cfg))

	txn := APITransaction{
		ID: "t1", Name: "下单流程", Level: "critical",
		Vars: map[string]string{"base": srv.URL},
		Steps: []APIStep{
			{Name: "登录", Method: "POST", URL: "{{base}}/login", JSONPath: "token", Extract: map[string]string{"token": "token"}},
			{Name: "下单", Method: "POST", URL: "{{base}}/order", Headers: map[string]string{"Authorization": "Bearer {{token}}"}, JSONPath: "code", JSONExpect: "0"},
		},
	}
	res := ar.runTxn(txn)
	if !res.OK || res.FailedStep != -1 {
		t.Fatalf("事务应全过，实际 OK=%v FailedStep=%d steps=%+v", res.OK, res.FailedStep, res.Steps)
	}
	if len(res.Steps) != 2 || !res.Steps[0].OK || !res.Steps[1].OK {
		t.Fatalf("两步都应成功: %+v", res.Steps)
	}

	// 提取不到 token（路径错误）→ {{token}} 保留原样 → 第二步 401 失败于步 index 1
	txn.Steps[0].Extract = map[string]string{"token": "nonexist_path"}
	res2 := ar.runTxn(txn)
	if res2.OK || res2.FailedStep != 1 {
		t.Fatalf("第二步应因缺 token 而失败于步 1，实际 OK=%v FailedStep=%d", res2.OK, res2.FailedStep)
	}
}
