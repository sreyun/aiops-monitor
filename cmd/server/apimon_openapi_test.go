package main

import "testing"

// TestParseOpenAPI 验证 OpenAPI3(servers) / Swagger2(schemes+host+basePath) 解析与基址覆盖。
func TestParseOpenAPI(t *testing.T) {
	spec3 := `{"servers":[{"url":"https://api.x.com/v1"}],"paths":{"/login":{"post":{"operationId":"login"}},"/users":{"get":{"summary":"列出用户"}}}}`
	eps, err := parseOpenAPI([]byte(spec3), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 {
		t.Fatalf("应解析 2 接口，得 %d", len(eps))
	}
	// 路径排序后 /login 在 /users 前
	if eps[0].URL != "https://api.x.com/v1/login" || eps[0].Method != "POST" || eps[0].Name != "login" {
		t.Fatalf("接口0 错误: %+v", eps[0])
	}
	if eps[1].Name != "列出用户" || eps[1].Method != "GET" {
		t.Fatalf("接口1 应回落到 summary: %+v", eps[1])
	}
	// baseURL 覆盖规范内基址
	eps2, _ := parseOpenAPI([]byte(spec3), "http://localhost:8080")
	if eps2[0].URL != "http://localhost:8080/login" {
		t.Fatalf("baseURL 覆盖失败: %s", eps2[0].URL)
	}
	// Swagger 2：schemes + host + basePath 推断基址
	spec2 := `{"schemes":["https"],"host":"api.y.com","basePath":"/api","paths":{"/ping":{"get":{}}}}`
	eps3, _ := parseOpenAPI([]byte(spec2), "")
	if len(eps3) != 1 || eps3[0].URL != "https://api.y.com/api/ping" {
		t.Fatalf("Swagger2 基址推断失败: %+v", eps3)
	}
	// 非法 JSON 应报错
	if _, err := parseOpenAPI([]byte("not json"), ""); err == nil {
		t.Error("非法 JSON 应返回错误")
	}
}
