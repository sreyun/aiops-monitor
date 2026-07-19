package main

import (
	"path/filepath"
	"testing"

	"aiops-monitor/shared"
)

// TestParsePromText 验证 Prometheus 文本解析：注释/TYPE 跳过、无标签/多标签、时间戳、NaN 跳过、
// 非法值跳过、+Inf 保留、extra 标签合并。
func TestParsePromText(t *testing.T) {
	body := []byte(`# HELP mysql_up Whether the server is up.
# TYPE mysql_up gauge
mysql_up 1
mysql_global_status_threads_connected{instance="db1"} 42
node_cpu_seconds_total{cpu="0",mode="idle"} 12345.6 1700000000000
weird_line not_a_number
some_nan{x="y"} NaN
some_inf +Inf
`)
	samples := parsePromText(body, 1700000000000, map[string]string{"job": "mysql"})
	idx := map[string]shared.LabeledSample{}
	for _, s := range samples {
		k := s.Name
		if s.Labels["instance"] != "" {
			k += "/" + s.Labels["instance"]
		}
		idx[k] = s
	}
	if s, ok := idx["mysql_up"]; !ok || s.Value != 1 || s.Labels["job"] != "mysql" {
		t.Fatalf("mysql_up 解析错误: %+v", s)
	}
	if s, ok := idx["mysql_global_status_threads_connected/db1"]; !ok || s.Value != 42 || s.Labels["instance"] != "db1" {
		t.Fatalf("threads_connected 解析错误: %+v", s)
	}
	if s, ok := idx["node_cpu_seconds_total"]; !ok || s.Labels["cpu"] != "0" || s.Labels["mode"] != "idle" {
		t.Fatalf("node_cpu 标签错误: %+v", s)
	}
	for _, s := range samples {
		if s.Name == "some_nan" {
			t.Error("NaN 样本应被跳过")
		}
		if s.Name == "weird_line" {
			t.Error("非法值行应被跳过")
		}
	}
	found := false
	for _, s := range samples {
		if s.Name == "some_inf" {
			found = true
		}
	}
	if !found {
		t.Error("+Inf 应保留")
	}
}

// TestParsePromLabels 验证标签解析含转义（\" \\ \n）。
func TestParsePromLabels(t *testing.T) {
	m := parsePromLabels(`a="1",b="two words",c="he said \"hi\"\nnext"`)
	if m["a"] != "1" || m["b"] != "two words" || m["c"] != "he said \"hi\"\nnext" {
		t.Fatalf("标签转义解析错误: %+v", m)
	}
}

// TestScrapeTargetPersistence 验证抓取目标（含 Labels/Headers）+ remote_write 令牌落盘。
func TestScrapeTargetPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.json")
	cs, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := cs.UpsertScrapeTarget(ScrapeTarget{
		Name: "mysql", URL: "http://exporter:9104/metrics", IntervalSec: 30,
		Labels: map[string]string{"env": "prod"}, Headers: map[string]string{"Authorization": "Bearer x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cs2, _ := NewConfigStore(path, nil)
	got := cs2.ScrapeTargets()
	if len(got) != 1 || got[0].ID != saved.ID || got[0].URL != "http://exporter:9104/metrics" {
		t.Fatalf("抓取目标未落盘: %+v", got)
	}
	if got[0].Labels["env"] != "prod" || got[0].Headers["Authorization"] != "Bearer x" {
		t.Fatalf("标签/请求头未落盘: %+v", got[0])
	}
	if err := cs2.SetPromWriteToken("secret123"); err != nil {
		t.Fatal(err)
	}
	cs3, _ := NewConfigStore(path, nil)
	if cs3.Get().PromWriteToken != "secret123" {
		t.Fatal("remote_write 令牌未落盘")
	}
}
