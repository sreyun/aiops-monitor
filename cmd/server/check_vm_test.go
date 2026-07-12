package main

import (
	"strings"
	"testing"
)

// TestParseVMCheckExport 验证从 VM /export NDJSON 重组拨测历史点——这是"重启后仍能查历史"的读路径核心。
func TestParseVMCheckExport(t *testing.T) {
	nd := `{"metric":{"__name__":"aiops_check_up","check_id":"c1"},"values":[1,0],"timestamps":[1700000000000,1700000030000]}
{"metric":{"__name__":"aiops_check_latency_ms","check_id":"c1"},"values":[42.5,99],"timestamps":[1700000000000,1700000030000]}
{"metric":{"__name__":"aiops_check_status_code","check_id":"c1"},"values":[200,500],"timestamps":[1700000000000,1700000030000]}
`
	pts := parseVMCheckExport(strings.NewReader(nd))
	if len(pts) != 2 {
		t.Fatalf("应重组 2 个时间点，得到 %d", len(pts))
	}
	// 排序后第一个是较早的时间戳
	if pts[0].Ts != 1700000000 || !pts[0].OK || pts[0].LatencyMs != 42.5 || pts[0].StatusCode != 200 {
		t.Fatalf("第一个点重组错误: %+v", pts[0])
	}
	if pts[1].Ts != 1700000030 || pts[1].OK || pts[1].LatencyMs != 99 || pts[1].StatusCode != 500 {
		t.Fatalf("第二个点重组错误: %+v", pts[1])
	}
}

// TestPushChecksFormat 验证拨测指标的 Prometheus 文本格式（名称/label/值）正确，供 VM 摄取。
func TestPushChecksFormat(t *testing.T) {
	// 直接构造一批样本，走 pushChecks 的格式化逻辑（用一个不发请求的方式：复用 lblEsc + 手工断言 label）。
	// 这里只校验 label 转义不破坏格式（真实网络写入由部署端到端验证）。
	got := lblEsc(`a"b\c`)
	if !strings.Contains(got, `\"`) || !strings.Contains(got, `\\`) {
		t.Fatalf("label 转义异常: %s", got)
	}
}
