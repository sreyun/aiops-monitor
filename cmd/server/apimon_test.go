package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAPIEndpointToCheck 验证接口→高级拨测的字段映射（复用 probeHTTPAdvanced 的关键）。
func TestAPIEndpointToCheck(t *testing.T) {
	ep := APIEndpoint{
		ID: "ep1", Name: "登录", URL: "https://x/login", Method: "POST",
		Headers: map[string]string{"X-Api-Key": "k"}, Body: `{"u":"a"}`,
		ExpectStatus: 200, ExpectKeyword: "ok", JSONPath: "code", JSONExpect: "0",
	}
	c := ep.toCheck()
	if !c.Advanced || c.Type != "http" {
		t.Fatal("应为 http 高级拨测")
	}
	if c.Target != ep.URL || c.Method != "POST" || c.Body != ep.Body {
		t.Fatal("URL/方法/请求体未正确映射")
	}
	if c.Headers["X-Api-Key"] != "k" || c.ExpectStatus != 200 || c.ExpectKeyword != "ok" {
		t.Fatal("头/状态码/关键字未正确映射")
	}
	if c.JSONPath != "code" || c.JSONExpect != "0" {
		t.Fatal("JSON 断言未正确映射")
	}
}

// TestPushAPIFormat 验证 pushAPI 生成的 Prometheus 文本含 aiops_api_* 指标族与 api_id/system/endpoint 标签。
func TestPushAPIFormat(t *testing.T) {
	var body string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
		body = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	v := &vmWriter{httpc: ts.Client()}
	v.pushAPI(ts.URL, []vmAPISample{{
		apiID: "ep1", system: "订单系统", endpoint: "下单",
		ts: 1700000000, ok: true, latencyMs: 42.5, statusCode: 200,
		dnsMs: 1.2, tcpMs: 2.3, tlsMs: 3.4, ttfbMs: 40.1, certDays: 20, respBytes: 512,
	}})

	for _, want := range []string{
		`aiops_api_up{api_id="ep1",system="订单系统",endpoint="下单"} 1 1700000000000`,
		"aiops_api_latency_ms{", "aiops_api_status_code{", "aiops_api_dns_ms{",
		"aiops_api_tcp_ms{", "aiops_api_tls_ms{", "aiops_api_ttfb_ms{",
		"aiops_api_cert_days{", "aiops_api_resp_bytes{",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("推送体缺少 %q\n实际:\n%s", want, body)
		}
	}
}

// TestParseVMAPIExport 验证把 VM /export NDJSON 按时间戳重组为历史点。
func TestParseVMAPIExport(t *testing.T) {
	nd := `{"metric":{"__name__":"aiops_api_up","api_id":"ep1"},"values":[1,0],"timestamps":[1700000000000,1700000030000]}
{"metric":{"__name__":"aiops_api_latency_ms","api_id":"ep1"},"values":[40,55],"timestamps":[1700000000000,1700000030000]}
{"metric":{"__name__":"aiops_api_status_code","api_id":"ep1"},"values":[200,500],"timestamps":[1700000000000,1700000030000]}`
	pts := parseVMAPIExport(strings.NewReader(nd))
	if len(pts) != 2 {
		t.Fatalf("应重组出 2 个时间点，实际 %d", len(pts))
	}
	if !pts[0].OK || pts[0].LatencyMs != 40 || pts[0].StatusCode != 200 {
		t.Fatalf("首点重组错误: %+v", pts[0])
	}
	if pts[1].OK || pts[1].LatencyMs != 55 || pts[1].StatusCode != 500 {
		t.Fatalf("次点重组错误: %+v", pts[1])
	}
}

// TestAPIRunnerProbe 端到端跑一次接口探测：命中 httptest 服务 → 记录实时状态（OK+延迟）。
func TestAPIRunnerProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Token") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()

	cfg := newTestConfigStore(t)
	cr := newCheckRunner(cfg, NewStore(), nil, "")
	ar := newAPIRunner(cr, cfg, NewStore(), nil, newVMWriter(cfg)) // VM 未启用 → enqueueAPI 空跑

	sys := APISystem{ID: "s1", Name: "订单系统", Level: "critical"}
	ep := APIEndpoint{
		ID: "ep1", Name: "查询", URL: srv.URL, Method: "GET",
		Headers: map[string]string{"X-Token": "secret"},
		ExpectStatus: 200, JSONPath: "code", JSONExpect: "0", Enabled: true,
	}
	ar.probe(sys, ep)

	st := ar.statusSnapshot()
	s, ok := st["ep1"]
	if !ok {
		t.Fatal("探测后应记录实时状态")
	}
	if !s.OK {
		t.Fatalf("接口应判为正常，实际 msg=%s", s.Message)
	}
	if s.StatusCode != 200 || s.LatencyMs <= 0 {
		t.Fatalf("状态码/延迟异常: code=%d lat=%f", s.StatusCode, s.LatencyMs)
	}
	if s.System != "订单系统" || s.Name != "查询" {
		t.Fatal("业务系统/接口名未记录")
	}
}
