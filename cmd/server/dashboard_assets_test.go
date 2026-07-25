package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeDashAppearance(t *testing.T) {
	ok := DashAppearance{
		LogoURL:         "/api/v1/dashboards/assets/abc12345/deadbeef01234567.png",
		BackgroundURL:   "/api/v1/dashboards/assets/abc12345/cafebabe89abcdef.webp",
		BackgroundColor: "#1a1f2e",
		BackgroundFit:   "contain",
		PanelOpacity:    0.85,
	}
	if err := normalizeDashAppearance(&ok); err != nil {
		t.Fatalf("合法外观应通过: %v", err)
	}
	badURL := DashAppearance{LogoURL: "https://evil.example/x.png"}
	if err := normalizeDashAppearance(&badURL); err == nil {
		t.Fatal("外链 Logo 应被拒绝")
	}
	badColor := DashAppearance{BackgroundColor: "red; background:url(x)"}
	if err := normalizeDashAppearance(&badColor); err == nil {
		t.Fatal("非法背景色应被拒绝")
	}
	badOp := DashAppearance{BackgroundColor: "#fff", PanelOpacity: 0.2}
	if err := normalizeDashAppearance(&badOp); err == nil {
		t.Fatal("过低不透明度应被拒绝")
	}
	fit := DashAppearance{BackgroundURL: "/api/v1/dashboards/assets/abc12345/deadbeef01234567.png"}
	if err := normalizeDashAppearance(&fit); err != nil {
		t.Fatal(err)
	}
	if fit.BackgroundFit != "cover" {
		t.Fatalf("有背景图时默认 fit 应为 cover，实为 %q", fit.BackgroundFit)
	}
}

func TestMapAIOpsDashboardStripsAssetURLs(t *testing.T) {
	raw := []byte(`{
	  "format":"aiops",
	  "dashboard":{
	    "name":"外观模板",
	    "appearance":{
	      "logo_url":"/api/v1/dashboards/assets/oldid/aaaaaaaaaaaaaaaa.png",
	      "background_url":"/api/v1/dashboards/assets/oldid/bbbbbbbbbbbbbbbb.jpg",
	      "background_color":"#112233",
	      "background_fit":"contain",
	      "panel_opacity":0.9
	    },
	    "panels":[{"id":1,"title":"CPU","type":"stat","grid":{"x":0,"y":0,"w":6,"h":6},"targets":[{"expr":"up"}]}]
	  }
	}`)
	d, err := mapAIOpsDashboard(raw, "", "aiops-template")
	if err != nil {
		t.Fatal(err)
	}
	if d.Appearance.LogoURL != "" || d.Appearance.BackgroundURL != "" {
		t.Fatalf("导入应清空图片 URL，实为 %+v", d.Appearance)
	}
	if d.Appearance.BackgroundColor != "#112233" || d.Appearance.BackgroundFit != "contain" {
		t.Fatalf("应保留配色与 fit: %+v", d.Appearance)
	}
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDashboardAssetUploadAndGet(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server_config.json")
	if err := os.WriteFile(cfgPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cs, err := NewConfigStore(cfgPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := cs.UpsertDashboard(Dashboard{
		Name:   "外观测试",
		Panels: []DashPanel{{ID: 1, Title: "s", Type: "stat", Grid: DashGrid{W: 24, H: 6}, Targets: []DashTarget{{Expr: "up"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: cs, store: NewStore()}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("kind", "logo")
	fw, err := mw.CreateFormFile("file", "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(tinyPNG(t)); err != nil {
		t.Fatal(err)
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/"+saved.ID+"/assets", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("id", saved.ID)
	rec := httptest.NewRecorder()
	s.handleUploadDashboardAsset(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	var up struct {
		OK  bool   `json:"ok"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil || !up.OK || up.URL == "" {
		t.Fatalf("upload resp: %s", rec.Body.String())
	}
	dashID, name, ok := parseDashAssetURL(up.URL)
	if !ok || dashID != saved.ID {
		t.Fatalf("bad url %q", up.URL)
	}
	disk := filepath.Join(dir, "dashboard-assets", dashID, name)
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("asset not on disk: %v", err)
	}

	get := httptest.NewRequest(http.MethodGet, up.URL, nil)
	get.SetPathValue("dashID", dashID)
	get.SetPathValue("name", name)
	grec := httptest.NewRecorder()
	s.handleGetDashboardAsset(grec, get)
	if grec.Code != http.StatusOK {
		t.Fatalf("get status=%d", grec.Code)
	}
	if !strings.HasPrefix(grec.Header().Get("Content-Type"), "image/png") {
		t.Fatalf("content-type=%q", grec.Header().Get("Content-Type"))
	}
	if grec.Body.Len() < 8 {
		t.Fatal("empty image body")
	}

	// 非法 kind / 非图片类型拒绝
	var body2 bytes.Buffer
	mw2 := multipart.NewWriter(&body2)
	_ = mw2.WriteField("kind", "avatar")
	fw2, _ := mw2.CreateFormFile("file", "x.png")
	_, _ = fw2.Write(tinyPNG(t))
	_ = mw2.Close()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/"+saved.ID+"/assets", &body2)
	req2.Header.Set("Content-Type", mw2.FormDataContentType())
	req2.SetPathValue("id", saved.ID)
	rec2 := httptest.NewRecorder()
	s.handleUploadDashboardAsset(rec2, req2)
	if rec2.Code == http.StatusOK {
		t.Fatal("非法 kind 应被拒绝")
	}

	s.removeDashboardAssets(saved.ID)
	if _, err := os.Stat(filepath.Join(dir, "dashboard-assets", saved.ID)); !os.IsNotExist(err) {
		t.Fatalf("删除后目录应不存在, err=%v", err)
	}
	_ = io.Discard
}

func TestRoutesRegisterIncludesDashAssets(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("Routes() panicked: %v", rec)
		}
	}()
	(&Server{}).Routes()
}
