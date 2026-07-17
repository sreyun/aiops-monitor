package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// 真机上会踩、但"标准 DMTF 夹具"永远测不出来的固件怪癖。
// 这些都是导致整块数据静默消失的元凶，各配一条回归。

func TestRedfishNumToleratesNA(t *testing.T) {
	// 华为 iBMC 在未配阈值的传感器上回字符串 "N/A"，而 schema 说是 number。
	// 用 float64 直解会让**整份 Thermal** 报错，风扇和温度一起没。
	var v struct {
		A redfishNum `json:"A"`
		B redfishNum `json:"B"`
		C redfishNum `json:"C"`
		D redfishNum `json:"D"`
		E redfishNum `json:"E"`
	}
	raw := `{"A":42.5,"B":"N/A","C":null,"D":"","E":"57"}`
	if err := jsonUnmarshal(raw, &v); err != nil {
		t.Fatalf("含 N/A 的文档不应报错: %v", err)
	}
	if v.A.f() != 42.5 {
		t.Errorf("A = %v, want 42.5", v.A.f())
	}
	for name, got := range map[string]float64{"B": v.B.f(), "C": v.C.f(), "D": v.D.f()} {
		if got != 0 {
			t.Errorf("%s = %v, want 0（无法识别的值按缺省处理）", name, got)
		}
	}
	if v.E.f() != 57 { // 有的固件把数字包成字符串
		t.Errorf("E = %v, want 57（字符串包裹的数字也要认）", v.E.f())
	}
}

// huaweiThermalNARoutes：温度阈值是 "N/A" 字符串 —— 真机形态。
func huaweiThermalNARoutes() map[string]string {
	r := huaweiRoutes()
	r["/redfish/v1/Chassis/1/Thermal"] = `{
		"Temperatures":[
			{"Name":"Inlet Temp","ReadingCelsius":27,"Status":{"Health":"OK","State":"Enabled"},
			 "UpperThresholdNonCritical":"N/A","UpperThresholdCritical":"N/A","UpperThresholdFatal":"N/A"},
			{"Name":"CPU1 Core Rem","ReadingCelsius":58,"Status":{"Health":"OK","State":"Enabled"},
			 "UpperThresholdNonCritical":85,"UpperThresholdCritical":"N/A","UpperThresholdFatal":95}],
		"Fans":[
			{"Name":"Fan Module1 Front","Reading":4920,"ReadingUnits":"RPM","Status":{"Health":"OK","State":"Enabled"},
			 "LowerThresholdCritical":"N/A","UpperThresholdCritical":"N/A"},
			{"Name":"Fan Module2 Front","Reading":5100,"ReadingUnits":"RPM","Status":{"Health":"OK","State":"Enabled"}}]}`
	return r
}

func TestHuaweiThermalSurvivesNAThresholds(t *testing.T) {
	srv := serveRoutes(t, huaweiThermalNARoutes())
	defer srv.Close()

	rc := newRedfishCollector(nil, "h1", "fp")
	snap := rc.collectOne(RedfishTarget{Name: "ibmc", URL: srv.URL, Username: "u", Password: "p"})
	if snap.Error != "" {
		t.Fatalf("collectOne error = %q", snap.Error)
	}
	// 关键：一个 "N/A" 字段不能让整份 Thermal 报废
	if len(snap.Fans) != 2 {
		t.Fatalf("风扇 = %d, want 2（阈值里的 \"N/A\" 不能让风扇整体消失）: %+v", len(snap.Fans), snap.Fans)
	}
	if snap.Fans[0].RPM != 4920 {
		t.Errorf("风扇转速 = %d, want 4920（读数在 Reading 字段）", snap.Fans[0].RPM)
	}
	if len(snap.Temps) != 2 {
		t.Fatalf("温度 = %d, want 2: %+v", len(snap.Temps), snap.Temps)
	}
	if snap.Temps[0].Reading != 27 {
		t.Errorf("温度读数 = %v, want 27", snap.Temps[0].Reading)
	}
	// "N/A" 的阈值应落成 0（前端显示 "-"），而不是让这条传感器消失
	if snap.Temps[0].UpperCritical != 0 {
		t.Errorf("N/A 阈值 = %v, want 0", snap.Temps[0].UpperCritical)
	}
	// UpperThresholdNonCritical 是 DMTF 里 Caution 的正式名，必须认
	if snap.Temps[1].UpperCaution != 85 {
		t.Errorf("告警阈值 = %v, want 85（来自 UpperThresholdNonCritical）", snap.Temps[1].UpperCaution)
	}
	// UpperThresholdCritical 是 "N/A" 时回落到 UpperThresholdFatal
	if snap.Temps[1].UpperCritical != 95 {
		t.Errorf("严重阈值 = %v, want 95（回落到 UpperThresholdFatal）", snap.Temps[1].UpperCritical)
	}
}

// ---- Dell iDRAC8：DIMM id 里带字面 '#' ----

func TestRfEscapeODataID(t *testing.T) {
	cases := map[string]string{
		// iDRAC8 真实形态：'#' 是 URL fragment 分隔符，不转义会让请求被截断
		"/redfish/v1/Systems/System.Embedded.1/Memory/iDRAC.Embedded.1#DIMMSLOTA1": "/redfish/v1/Systems/System.Embedded.1/Memory/iDRAC.Embedded.1%23DIMMSLOTA1",
		// 已经转义过的不能再转一次
		"/redfish/v1/Systems/System.Embedded.1/Memory/iDRAC.Embedded.1%23DIMMSLOTA1": "/redfish/v1/Systems/System.Embedded.1/Memory/iDRAC.Embedded.1%23DIMMSLOTA1",
		// iDRAC9 无 '#'，原样不动
		"/redfish/v1/Systems/System.Embedded.1/Memory/DIMM.Socket.A1": "/redfish/v1/Systems/System.Embedded.1/Memory/DIMM.Socket.A1",
	}
	for in, want := range cases {
		if got := rfEscapeODataID(in); got != want {
			t.Errorf("rfEscapeODataID(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

// idrac8MemRoutes 复刻 iDRAC8：DIMM 成员 id 含字面 '#'。
//
// 路由键用**解码后**的字面 '#'：net/http 服务端会把 %23 解回 '#' 再填进
// r.URL.Path。于是这条测试正好卡住真实行为——
//   - 采集端转义了 → 请求 %23 → 服务端解码成 '#' → 命中路由 → 拿到 DIMM
//   - 采集端没转义 → '#' 被当 fragment 剪掉 → 请求 /Memory/iDRAC.Embedded.1 → 404 → 全丢
func idrac8MemRoutes() map[string]string {
	r := idrac8Routes()
	r["/redfish/v1/Systems/System.Embedded.1"] = `{
		"Status":{"Health":"OK","State":"Enabled"},
		"Manufacturer":"Dell Inc.","Model":"PowerEdge R730","SKU":"3X9L1N2","BiosVersion":"2.14.0","PowerState":"On",
		"MemorySummary":{"TotalSystemMemoryGiB":128},
		"Memory":{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Memory"},
		"Processors":{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Processors"},
		"Links":{"Chassis":[{"@odata.id":"/redfish/v1/Chassis/System.Embedded.1"}]}}`
	r["/redfish/v1/Systems/System.Embedded.1/Memory"] = `{"Members":[
		{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Memory/iDRAC.Embedded.1#DIMMSLOTA1"},
		{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Memory/iDRAC.Embedded.1#DIMMSLOTB1"}]}`
	r["/redfish/v1/Systems/System.Embedded.1/Memory/iDRAC.Embedded.1#DIMMSLOTA1"] = `{
		"Name":"DIMM A1","Id":"iDRAC.Embedded.1#DIMMSLOTA1","CapacityMiB":16384,
		"MemoryDeviceType":"DDR4","OperatingSpeedMhz":2400,"DeviceLocator":"A1",
		"Manufacturer":"Samsung","PartNumber":"M393A2G40DB0","SerialNumber":"12AB34CD","RankCount":2,
		"Status":{"Health":"OK","State":"Enabled"}}`
	r["/redfish/v1/Systems/System.Embedded.1/Memory/iDRAC.Embedded.1#DIMMSLOTB1"] = `{
		"Name":"DIMM B1","Id":"iDRAC.Embedded.1#DIMMSLOTB1","CapacityMiB":16384,
		"MemoryDeviceType":"DDR4","OperatingSpeedMhz":2400,"DeviceLocator":"B1",
		"Manufacturer":"Samsung","PartNumber":"M393A2G40DB0","SerialNumber":"56EF78AB","RankCount":2,
		"Status":{"Health":"OK","State":"Enabled"}}`
	return r
}

func TestIDRAC8DIMMsWithHashInID(t *testing.T) {
	srv := serveRoutes(t, idrac8MemRoutes())
	defer srv.Close()

	rc := newRedfishCollector(nil, "h5", "fp")
	snap := rc.collectOne(RedfishTarget{Name: "idrac8", URL: srv.URL, Username: "u", Password: "p"})
	if snap.Error != "" {
		t.Fatalf("collectOne error = %q", snap.Error)
	}
	// 这是 R730"缺少内存"的根因：'#' 未转义 → 请求被截断 → 404 → 全丢
	if len(snap.Memory.DIMMs) != 2 {
		t.Fatalf("DIMM = %d, want 2（id 里的 '#' 必须转义成 %%23，否则请求被截断到 /Memory/iDRAC.Embedded.1）: %+v",
			len(snap.Memory.DIMMs), snap.Memory.DIMMs)
	}
	d := snap.Memory.DIMMs[0]
	if d.Slot != "A1" || d.CapacityGB != 16 || d.SerialNumber != "12AB34CD" {
		t.Errorf("DIMM[0] = %+v", d)
	}
}

// 记录 net/http 的真实行为，防止有人"顺手"把转义去掉。
func TestHashInPathTruncatesRequest(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	rc := newRedfishCollector(nil, "h", "fp")
	var dst map[string]any
	_ = rc.rfGetRaw(srv.Client(), srv.URL, "", "", "/redfish/v1/Memory/iDRAC.Embedded.1#DIMMSLOTA1", &dst)

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("请求数 = %d", len(seen))
	}
	// 转义后服务端应看到完整路径；若回归成字面 '#'，这里会变成 /redfish/v1/Memory/iDRAC.Embedded.1
	if !strings.HasSuffix(seen[0], "DIMMSLOTA1") {
		t.Errorf("服务端收到的路径 = %q，'#' 之后被截断了 —— 转义失效", seen[0])
	}
}

// jsonUnmarshal 是测试里的小helper，避免每处都 import encoding/json。
func jsonUnmarshal(raw string, dst any) error { return json.Unmarshal([]byte(raw), dst) }
