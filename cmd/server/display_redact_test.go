package main

import (
	"strings"
	"testing"
)

func TestHostDisplayLabelNeverShowsID(t *testing.T) {
	got := hostDisplayLabel("", "", "ABCDEF0123456789")
	if got != "未知主机" {
		t.Fatalf("want placeholder, got %q", got)
	}
	got = hostDisplayLabel("web-1", "10.0.0.1", "ABCDEF0123456789")
	if got != "web-1 (10.0.0.1)" {
		t.Fatalf("got %q", got)
	}
}

func TestRedactUserFacingText(t *testing.T) {
	in := "请在 hermes 上检查主机 ABCDEF0123456789 的 CPU"
	out := redactUserFacingText(in, map[string]string{"ABCDEF0123456789": "web-1 (10.0.0.1)"})
	if strings.Contains(strings.ToLower(out), "hermes") {
		t.Fatalf("hermes leaked: %q", out)
	}
	if strings.Contains(out, "ABCDEF0123456789") {
		t.Fatalf("host id leaked: %q", out)
	}
	if !strings.Contains(out, "web-1 (10.0.0.1)") {
		t.Fatalf("label missing: %q", out)
	}
	if !strings.Contains(out, "智能运维服务") {
		t.Fatalf("brand rewrite missing: %q", out)
	}

	// Historical shortID (8 hex) in delete messages
	short := redactUserFacingText("删除主机 ABCDEF01", map[string]string{"ABCDEF0123456789": "web-1 (10.0.0.1)"})
	if strings.Contains(short, "ABCDEF01") {
		t.Fatalf("short id leaked: %q", short)
	}
	if !strings.Contains(short, "web-1 (10.0.0.1)") {
		t.Fatalf("short id label missing: %q", short)
	}

	orphan := redactUserFacingText("关闭远程终端 deadbeefcafe", nil)
	if strings.Contains(orphan, "deadbeefcafe") {
		t.Fatalf("orphan hex leaked: %q", orphan)
	}
}
