package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHandleDownload 锁定 /dl 缓存下发的关键行为：ETag、条件 GET→304、Range→206、
// 防目录穿越。这些是"重装/多机不重复全量下载 + 断点续传"能生效的前提。
func TestHandleDownload(t *testing.T) {
	dir := t.TempDir()
	content := []byte("fake-agent-binary-0123456789")
	if err := os.WriteFile(filepath.Join(dir, "aiops-agent.exe"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{distDir: dir}

	// 1) 完整 GET → 200 + ETag + Cache-Control + 原文
	rw := httptest.NewRecorder()
	s.handleDownload(rw, httptest.NewRequest("GET", "/dl/aiops-agent.exe", nil))
	if rw.Code != http.StatusOK {
		t.Fatalf("完整 GET 应 200，得 %d", rw.Code)
	}
	etag := rw.Header().Get("ETag")
	if etag == "" {
		t.Fatal("缺少 ETag")
	}
	if rw.Header().Get("Cache-Control") == "" {
		t.Fatal("缺少 Cache-Control")
	}
	if !bytes.Equal(rw.Body.Bytes(), content) {
		t.Fatal("响应体与文件不一致")
	}

	// 2) 条件 GET(If-None-Match 命中) → 304，不回 body（多机/重装省带宽的关键）
	req2 := httptest.NewRequest("GET", "/dl/aiops-agent.exe", nil)
	req2.Header.Set("If-None-Match", etag)
	rw2 := httptest.NewRecorder()
	s.handleDownload(rw2, req2)
	if rw2.Code != http.StatusNotModified {
		t.Fatalf("ETag 命中应 304，得 %d", rw2.Code)
	}
	if rw2.Body.Len() != 0 {
		t.Fatal("304 不应有响应体")
	}

	// 3) Range → 206 partial（断点续传的基础）
	req3 := httptest.NewRequest("GET", "/dl/aiops-agent.exe", nil)
	req3.Header.Set("Range", "bytes=0-4")
	rw3 := httptest.NewRecorder()
	s.handleDownload(rw3, req3)
	if rw3.Code != http.StatusPartialContent {
		t.Fatalf("Range 应 206，得 %d", rw3.Code)
	}
	if !bytes.Equal(rw3.Body.Bytes(), content[:5]) {
		t.Fatalf("Range 分片内容错: %q", rw3.Body.Bytes())
	}

	// 4) 目录穿越 → 404（不能读到 distDir 之外）
	req4 := httptest.NewRequest("GET", "/dl/whatever", nil)
	req4.URL.Path = "/dl/../../etc/passwd"
	rw4 := httptest.NewRecorder()
	s.handleDownload(rw4, req4)
	if rw4.Code != http.StatusNotFound {
		t.Fatalf("穿越路径应 404，得 %d", rw4.Code)
	}

	// 5) 动态 SHA-256 清单与二进制严格对应，供安装器下载后校验。
	rw5 := httptest.NewRecorder()
	s.handleDownload(rw5, httptest.NewRequest("GET", "/dl/aiops-agent.exe.sha256", nil))
	if rw5.Code != http.StatusOK {
		t.Fatalf("checksum GET 应 200，得 %d", rw5.Code)
	}
	wantSum := fmt.Sprintf("%x", sha256.Sum256(content))
	if !strings.HasPrefix(rw5.Body.String(), wantSum+"  aiops-agent.exe") {
		t.Fatalf("checksum 内容错: %q", rw5.Body.String())
	}
}
