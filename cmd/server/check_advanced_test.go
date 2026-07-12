package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProbeHTTPAdvanced 验证 HTTP 高级拨测：自定义鉴权头、状态码/关键字/JSON 断言校验、分段计时。
func TestProbeHTTPAdvanced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" { // 静态鉴权头
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"code":0,"data":{"token":"abc"}}`)
			return
		}
		io.WriteString(w, "service is healthy")
	}))
	defer srv.Close()

	cr := newCheckRunner(newTestConfigStore(t), NewStore(), nil, "")

	// 1) 自定义头 + 关键字校验通过 + 有耗时
	r := cr.probeHTTPAdvanced(CustomCheck{Type: "http", Target: srv.URL, Advanced: true,
		Headers: map[string]string{"X-API-Key": "secret"}, ExpectKeyword: "healthy"})
	if !r.ok {
		t.Fatalf("含关键字应通过: %s", r.msg)
	}
	if r.totalMs <= 0 {
		t.Fatal("应记录总耗时")
	}

	// 2) 缺鉴权头 → 401 → 期望 200 失败
	if r2 := cr.probeHTTPAdvanced(CustomCheck{Type: "http", Target: srv.URL, Advanced: true, ExpectStatus: 200}); r2.ok {
		t.Fatal("缺鉴权头应失败")
	}

	// 3) POST + JSON 断言 code==0 通过
	r3 := cr.probeHTTPAdvanced(CustomCheck{Type: "http", Target: srv.URL, Advanced: true, Method: "POST", Body: "{}",
		Headers: map[string]string{"X-API-Key": "secret"}, JSONPath: "code", JSONExpect: "0"})
	if !r3.ok {
		t.Fatalf("JSON code==0 应通过: %s", r3.msg)
	}

	// 4) JSON 期望值不符 → 失败
	if r4 := cr.probeHTTPAdvanced(CustomCheck{Type: "http", Target: srv.URL, Advanced: true, Method: "POST", Body: "{}",
		Headers: map[string]string{"X-API-Key": "secret"}, JSONPath: "code", JSONExpect: "1"}); r4.ok {
		t.Fatal("JSON code==1 应失败（实际 0）")
	}

	// 5) 嵌套路径 data.token
	r5 := cr.probeHTTPAdvanced(CustomCheck{Type: "http", Target: srv.URL, Advanced: true, Method: "POST", Body: "{}",
		Headers: map[string]string{"X-API-Key": "secret"}, JSONPath: "data.token", JSONExpect: "abc"})
	if !r5.ok {
		t.Fatalf("嵌套 JSON 断言应通过: %s", r5.msg)
	}

	// 6) 关键字缺失 → 失败
	if r6 := cr.probeHTTPAdvanced(CustomCheck{Type: "http", Target: srv.URL, Advanced: true,
		Headers: map[string]string{"X-API-Key": "secret"}, ExpectKeyword: "不存在的词"}); r6.ok {
		t.Fatal("关键字缺失应失败")
	}
}

// TestJSONPathValue 验证 JSON 点路径取值（含 $ 前缀、嵌套、类型转字符串）。
func TestJSONPathValue(t *testing.T) {
	body := []byte(`{"code":0,"ok":true,"data":{"token":"abc","n":3.5}}`)
	cases := []struct {
		path, want string
		ok         bool
	}{
		{"code", "0", true},
		{"$.code", "0", true},
		{"ok", "true", true},
		{"data.token", "abc", true},
		{"data.n", "3.5", true},
		{"data.missing", "", false},
		{"nope", "", false},
	}
	for _, c := range cases {
		got, ok := jsonPathValue(body, c.path)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("jsonPathValue(%q)=(%q,%v) 期望 (%q,%v)", c.path, got, ok, c.want, c.ok)
		}
	}
}
