package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeOpenAPIDocURL(t *testing.T) {
	root, hint, err := normalizeOpenAPIDocURL("http://192.168.2.141:8080/doc.html#/home")
	if err != nil {
		t.Fatal(err)
	}
	if root != "http://192.168.2.141:8080" {
		t.Fatalf("root=%s", root)
	}
	if hint != "" {
		t.Fatalf("home should not be group hint, got %q", hint)
	}
	root2, hint2, err := normalizeOpenAPIDocURL("http://192.168.2.141:8080/doc.html#/SwaggerModels/activiti")
	if err != nil {
		t.Fatal(err)
	}
	if root2 != "http://192.168.2.141:8080" || hint2 != "activiti" {
		t.Fatalf("root=%s hint=%s", root2, hint2)
	}
	root3, _, err := normalizeOpenAPIDocURL("http://host/app/doc.html")
	if err != nil || root3 != "http://host/app" {
		t.Fatalf("context path root=%s err=%v", root3, err)
	}
}

func TestParseSwaggerResourcesAndSpringdoc(t *testing.T) {
	raw := `[{"name":"activiti","location":"/v2/api-docs?group=activiti","swaggerVersion":"2.0"}]`
	gs := parseSwaggerResources("http://h:8080", []byte(raw))
	if len(gs) != 1 || gs[0].Name != "activiti" || !strings.Contains(gs[0].URL, "group=activiti") {
		t.Fatalf("%+v", gs)
	}
	cfg := `{"urls":[{"name":"demo","url":"/v3/api-docs/demo"}]}`
	gs2 := parseSpringdocConfig("http://h:8080", []byte(cfg))
	if len(gs2) != 1 || gs2[0].URL != "http://h:8080/v3/api-docs/demo" {
		t.Fatalf("%+v", gs2)
	}
}

func TestFetchOpenAPIFromDocURL_Knife4j(t *testing.T) {
	spec := `{"openapi":"3.0.1","info":{"title":"Activiti API"},"paths":{"/flow":{"get":{"summary":"list"}}}}`
	mux := http.NewServeMux()
	mux.HandleFunc("/swagger-resources", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"name": "activiti", "location": "/v2/api-docs?group=activiti"},
		})
	})
	mux.HandleFunc("/v2/api-docs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("group") != "activiti" {
			http.Error(w, "bad group", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(spec))
	})
	mux.HandleFunc("/doc.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>knife4j</html>"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	res, err := fetchOpenAPIFromDocURL(ts.URL+"/doc.html#/SwaggerModels/activiti", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Spec, `"openapi"`) {
		t.Fatalf("spec missing: %s", res.Spec)
	}
	if res.SelectedGroup != "activiti" {
		t.Fatalf("group=%s", res.SelectedGroup)
	}
	if res.SuggestedName != "Activiti API" {
		t.Fatalf("name=%s", res.SuggestedName)
	}
	if res.SuggestedBase != ts.URL {
		t.Fatalf("base=%s want %s", res.SuggestedBase, ts.URL)
	}
}

func TestFetchOpenAPIFromDocURL_DirectV3(t *testing.T) {
	spec := `{"openapi":"3.0.2","info":{"title":"Demo"},"paths":{"/ping":{"get":{}}}}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/api-docs" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(spec))
	}))
	defer ts.Close()
	res, err := fetchOpenAPIFromDocURL(ts.URL+"/doc.html#/home", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.SuggestedName != "Demo" {
		t.Fatalf("%+v", res)
	}
}

func TestFetchOpenAPIFromDocURL_GatewaySwaggerConfig(t *testing.T) {
	spec := `{"openapi":"3.0.1","info":{"title":"activiti API"},"paths":{"/flow":{"get":{"summary":"list"}}}}`
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/api-docs/swagger-config", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"urls": []map[string]string{
				{"name": "activiti", "url": "/activiti/v3/api-docs", "contextPath": "/activiti"},
				{"name": "base", "url": "/base/v3/api-docs", "contextPath": "/base"},
			},
		})
	})
	mux.HandleFunc("/activiti/v3/api-docs", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(spec))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	res, err := fetchOpenAPIFromDocURL(ts.URL+"/doc.html#/home", "activiti", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.SelectedGroup != "activiti" {
		t.Fatalf("group=%s", res.SelectedGroup)
	}
	wantBase := ts.URL + "/activiti"
	if res.SuggestedBase != wantBase {
		t.Fatalf("base=%s want %s", res.SuggestedBase, wantBase)
	}
	if len(res.Groups) < 2 {
		t.Fatalf("expected multiple groups: %+v", res.Groups)
	}
}

func TestLooksLikeOpenAPIJSON(t *testing.T) {
	if !looksLikeOpenAPIJSON([]byte(`{"openapi":"3.0.0","paths":{}}`)) {
		t.Fatal("openapi")
	}
	if !looksLikeOpenAPIJSON([]byte(`{"swagger":"2.0","paths":{}}`)) {
		t.Fatal("swagger")
	}
	if looksLikeOpenAPIJSON([]byte(`{"name":"x"}`)) {
		t.Fatal("should reject")
	}
}
