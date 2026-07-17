package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 用假 BMC 覆盖两家厂商的路径差异。没有这层，华为的采集缺失只能等上了真机才发现。

func serveRoutes(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, ok := routes[r.URL.Path]; ok { // 查询参数($top)不参与路由匹配
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
}

// huaweiRoutes 复刻华为 iBMC(Kunpeng Server Board S920 S00 / TaiShan) 的真实布局：
//   - System.Storage 指向 /Storages（复数）
//   - 物理盘挂在 Chassis.Links.Drives
//   - 硬件事件在 Systems/1/LogServices/Log1，Managers 下只有 BMC 操作日志
//   - Oem.Huawei.ProcessorView / MemoryView 一次性返回 CPU/内存
func huaweiRoutes() map[string]string {
	return map[string]string{
		"/redfish/v1/Systems":    `{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`,
		"/redfish/v1/Chassis":    `{"Members":[{"@odata.id":"/redfish/v1/Chassis/1"}]}`,
		"/redfish/v1/Managers":   `{"Members":[{"@odata.id":"/redfish/v1/Managers/1"}]}`,
		"/redfish/v1/Managers/1": `{"Model":"iBMC","FirmwareVersion":"6.22.00.00"}`,
		"/redfish/v1/Systems/1": `{
			"Status":{"Health":"OK","State":"Enabled"},
			"Manufacturer":"Huawei","Model":"Kunpeng Server Board S920 S00",
			"SerialNumber":"2102312WPY10K9000456","BiosVersion":"1.79","PowerState":"On",
			"MemorySummary":{"TotalSystemMemoryGiB":256},
			"Storage":{"@odata.id":"/redfish/v1/Systems/1/Storages"},
			"Memory":{"@odata.id":"/redfish/v1/Systems/1/Memory"},
			"Processors":{"@odata.id":"/redfish/v1/Systems/1/Processors"},
			"LogServices":{"@odata.id":"/redfish/v1/Systems/1/LogServices"},
			"Oem":{"Huawei":{
				"ProcessorView":{"@odata.id":"/redfish/v1/Systems/1/ProcessorView"},
				"MemoryView":{"@odata.id":"/redfish/v1/Systems/1/MemoryView"}}},
			"Links":{"Chassis":[{"@odata.id":"/redfish/v1/Chassis/1"}]}}`,
		"/redfish/v1/Systems/1/ProcessorView": `{"Information":[
			{"Name":"CPU1","Model":"HiSilicon Kunpeng 920 5250","ProcessorType":"CPU",
			 "TotalCores":48,"TotalThreads":48,"MaxSpeedMHz":2600,"Temperature":51,
			 "Status":{"Health":"OK","State":"Enabled"}},
			{"Name":"CPU2","Model":"HiSilicon Kunpeng 920 5250","ProcessorType":"CPU",
			 "TotalCores":48,"TotalThreads":48,"MaxSpeedMHz":2600,"Temperature":49,
			 "Status":{"Health":"OK","State":"Enabled"}}]}`,
		"/redfish/v1/Systems/1/MemoryView": `{"Information":[
			{"Name":"DIMM000","DeviceLocator":"DIMM000 A0","CapacityMiB":32768,"MemoryDeviceType":"DDR4",
			 "OperatingSpeedMhz":2933,"Manufacturer":"Micron","PartNumber":"MTA18ASF4G72PDZ",
			 "SerialNumber":"2E3F4A5B","RankCount":2,"Status":{"Health":"OK","State":"Enabled"}},
			{"Name":"DIMM001","CapacityMiB":0,"Status":{"State":"Absent"}}]}`,
		"/redfish/v1/Systems/1/Storages": `{"Members":[{"@odata.id":"/redfish/v1/Systems/1/Storages/RAIDStorage0"}]}`,
		"/redfish/v1/Systems/1/Storages/RAIDStorage0": `{
			"Name":"RAIDStorage0",
			"StorageControllers":[{"Name":"RAID Card1 Controller","Model":"LSI SAS3508",
				"Manufacturer":"LSI","FirmwareVersion":"4.270.00-4382","SpeedGbps":12,
				"CacheSummary":{"TotalCacheSizeMiB":2048,"Status":{"Health":"OK"}},
				"Status":{"Health":"OK","State":"Enabled"}}],
			"Drives":[{"@odata.id":"/redfish/v1/Chassis/1/Drives/HDDPlaneDisk0"}],
			"Volumes":{"@odata.id":"/redfish/v1/Systems/1/Storages/RAIDStorage0/Volumes"}}`,
		"/redfish/v1/Systems/1/Storages/RAIDStorage0/Volumes": `{"Members":[{"@odata.id":"/redfish/v1/Systems/1/Storages/RAIDStorage0/Volumes/LogicalDrive0"}]}`,
		"/redfish/v1/Systems/1/Storages/RAIDStorage0/Volumes/LogicalDrive0": `{
			"Name":"LogicalDrive0","VolumeType":"RAID1","CapacityBytes":1600321314816,
			"Status":{"Health":"OK","State":"Enabled"}}`,
		"/redfish/v1/Chassis/1": `{
			"Thermal":{"@odata.id":"/redfish/v1/Chassis/1/Thermal"},
			"Power":{"@odata.id":"/redfish/v1/Chassis/1/Power"},
			"Links":{"Drives":[
				{"@odata.id":"/redfish/v1/Chassis/1/Drives/HDDPlaneDisk0"},
				{"@odata.id":"/redfish/v1/Chassis/1/Drives/HDDPlaneDisk1"}]}}`,
		"/redfish/v1/Chassis/1/Drives/HDDPlaneDisk0": `{
			"Name":"Disk0","Model":"HWE32P43016M002N","Manufacturer":"Huawei","SerialNumber":"032XYZ10J5",
			"Revision":"H2B1","CapacityBytes":1600321314816,"MediaType":"SSD","Protocol":"NVMe",
			"PredictedMediaLifeLeftPercent":99,"Status":{"Health":"OK","State":"Enabled"},
			"Oem":{"Huawei":{"Position":"Disk0"}}}`,
		"/redfish/v1/Chassis/1/Drives/HDDPlaneDisk1": `{
			"Name":"Disk1","Model":"HWE32P43016M002N","SerialNumber":"032XYZ10J6",
			"CapacityBytes":1600321314816,"MediaType":"SSD","Protocol":"NVMe","FailurePredicted":true,
			"Status":{"Health":"Warning","State":"Enabled"},"Oem":{"Huawei":{"Position":"Disk1"}}}`,
		"/redfish/v1/Chassis/1/Thermal": `{"Temperatures":[
			{"Name":"Inlet Temp","ReadingCelsius":23,"Status":{"Health":"OK"},
			 "UpperThresholdCaution":40,"UpperThresholdCritical":45}],
			"Fans":[{"Name":"FAN1 F","Reading":9600,"Status":{"Health":"OK","State":"Enabled"}}]}`,
		"/redfish/v1/Chassis/1/Power": `{
			"PowerControl":[{"Name":"System Power","PowerConsumedWatts":415}],
			"PowerSupplies":[{"Name":"PSU1","PowerInputWatts":415,"PowerCapacityWatts":1200,
				"LineInputVoltage":220,"Model":"PAC1200S12-CB","Manufacturer":"Huawei",
				"Status":{"Health":"OK","State":"Enabled"}}],
			"Redundancy":[{"Mode":"Sparing"}]}`,
		// Systems 下是硬件事件 —— 必须选中这个
		"/redfish/v1/Systems/1/LogServices": `{"Members":[{"@odata.id":"/redfish/v1/Systems/1/LogServices/Log1"}]}`,
		"/redfish/v1/Systems/1/LogServices/Log1": `{"Name":"Log Service",
			"Entries":{"@odata.id":"/redfish/v1/Systems/1/LogServices/Log1/Entries"}}`,
		"/redfish/v1/Systems/1/LogServices/Log1/Entries": `{"Members":[
			{"Id":"1","Created":"2026-07-17T10:00:00+08:00","Severity":"Critical",
			 "Message":"The fan is faulty.","Oem":{"Huawei":{"EventSubject":"FAN1"}}},
			{"Id":"2","Created":"2026-07-16T09:00:00+08:00","Message":"DIMM error",
			 "Oem":{"Huawei":{"Level":"Minor","EventSubject":"DIMM000"}}}]}`,
		// Managers 下全是 BMC 操作日志 —— 绝不能选中
		"/redfish/v1/Managers/1/LogServices": `{"Members":[
			{"@odata.id":"/redfish/v1/Managers/1/LogServices/OperateLog"},
			{"@odata.id":"/redfish/v1/Managers/1/LogServices/RunLog"},
			{"@odata.id":"/redfish/v1/Managers/1/LogServices/SecurityLog"}]}`,
		"/redfish/v1/Managers/1/LogServices/OperateLog": `{"Entries":{"@odata.id":"/redfish/v1/Managers/1/LogServices/OperateLog/Entries"}}`,
		"/redfish/v1/Managers/1/LogServices/OperateLog/Entries": `{"Members":[
			{"Id":"9","Created":"2026-07-17T11:00:00+08:00","Message":"admin logged in","Name":"Operate Log"}]}`,
		"/redfish/v1/UpdateService/FirmwareInventory": `{"Members":[]}`,
	}
}

// 华为 iBMC 的 Storage 属性指向 /Storages(复数)。此前代码硬拼 sysPath+"/Storage"
// → 404 → 硬盘/RAID卡/逻辑卷整片为空，正是现场"存储采不到"的根因。
func TestHuaweiStorageViaLinkNotConcat(t *testing.T) {
	srv := serveRoutes(t, huaweiRoutes())
	defer srv.Close()

	rc := newRedfishCollector(nil, "h1", "fp")
	snap := rc.collectOne(RedfishTarget{Name: "ibmc", URL: srv.URL, Username: "u", Password: "p"})
	if snap.Error != "" {
		t.Fatalf("collectOne error = %q", snap.Error)
	}

	if len(snap.RAID) != 1 || snap.RAID[0].Model != "LSI SAS3508" {
		t.Fatalf("RAID = %+v, want 1 个 LSI SAS3508（拼路径会拿不到）", snap.RAID)
	}
	if snap.RAID[0].CacheMB != 2048 {
		t.Errorf("RAID CacheMB = %v, want 2048", snap.RAID[0].CacheMB)
	}
	if len(snap.RAID[0].Volumes) != 1 || snap.RAID[0].Volumes[0].RAIDType != "RAID1" {
		t.Errorf("Volumes = %+v, want LogicalDrive0/RAID1（老固件只有 VolumeType）", snap.RAID[0].Volumes)
	}
	// 两块盘：Storage 成员给了 Disk0，Chassis.Links 给了 Disk0+Disk1 → 去重后应为 2
	if len(snap.Storage) != 2 {
		t.Fatalf("Storage = %d 块盘, want 2（Chassis.Links.Drives 合并去重）: %+v", len(snap.Storage), snap.Storage)
	}
	names := snap.Storage[0].Name + "," + snap.Storage[1].Name
	if !strings.Contains(names, "Disk0") || !strings.Contains(names, "Disk1") {
		t.Errorf("盘名 = %q, want 含 Disk0 与 Disk1", names)
	}
	if snap.Storage[0].Location != "Disk0" {
		t.Errorf("Location = %q, want Disk0（华为槽位在 Oem.Huawei.Position）", snap.Storage[0].Location)
	}
	if snap.Storage[0].LifeLeftPct != 99 {
		t.Errorf("LifeLeftPct = %v, want 99", snap.Storage[0].LifeLeftPct)
	}
	if !snap.Storage[1].SMARTWarn {
		t.Errorf("Disk1 应被标记 SMART 预测故障")
	}
}

// 硬件事件在 Systems/1/LogServices/Log1；Managers 下的 OperateLog 是 BMC 登录审计，
// 选错了等于把"谁登录了 BMC"当成硬件故障展示。
func TestHuaweiPicksSystemEventLogNotManagerOperateLog(t *testing.T) {
	srv := serveRoutes(t, huaweiRoutes())
	defer srv.Close()

	rc := newRedfishCollector(nil, "h1", "fp")
	tgt := RedfishTarget{Name: "ibmc", URL: srv.URL, Username: "u", Password: "p"}
	snap := rc.collectOne(tgt)

	if len(snap.Events) != 2 {
		t.Fatalf("Events = %d, want 2: %+v", len(snap.Events), snap.Events)
	}
	for _, e := range snap.Events {
		if strings.Contains(e.Message, "logged in") {
			t.Fatalf("选中了 BMC 操作日志而非硬件事件: %+v", e)
		}
	}
	rc.mu.Lock()
	pinned := rc.logPath[tgt.Name]
	rc.mu.Unlock()
	if !strings.HasSuffix(pinned, "/Log1") {
		t.Errorf("选中的日志服务 = %q, want Systems/1/LogServices/Log1", pinned)
	}
	// 归因来自 Oem.Huawei.EventSubject
	if snap.Events[0].Component != "FAN1" {
		t.Errorf("Events[0].Component = %q, want FAN1", snap.Events[0].Component)
	}
	// 华为的 Minor 必须归一成 Warning，否则前端按未知级别渲染成灰色"未知"
	if snap.Events[1].Severity != "Warning" {
		t.Errorf("Events[1].Severity = %q, want Warning（Oem.Huawei.Level=Minor 归一）", snap.Events[1].Severity)
	}
}

// ProcessorView/MemoryView：一次 GET 拿全，且带标准 schema 没有的 CPU 温度。
func TestHuaweiOemViews(t *testing.T) {
	srv := serveRoutes(t, huaweiRoutes())
	defer srv.Close()

	rc := newRedfishCollector(nil, "h1", "fp")
	snap := rc.collectOne(RedfishTarget{Name: "ibmc", URL: srv.URL, Username: "u", Password: "p"})

	if len(snap.CPUs) != 2 {
		t.Fatalf("CPUs = %d, want 2: %+v", len(snap.CPUs), snap.CPUs)
	}
	if snap.CPUs[0].TempC != 51 {
		t.Errorf("CPU 温度 = %v, want 51（此前 TempC 从未被赋值）", snap.CPUs[0].TempC)
	}
	if snap.CPUs[0].Cores != 48 || !strings.Contains(snap.CPUs[0].Model, "Kunpeng 920") {
		t.Errorf("CPU = %+v, want 48 核 Kunpeng 920", snap.CPUs[0])
	}
	// 空槽必须滤掉
	if len(snap.Memory.DIMMs) != 1 {
		t.Fatalf("DIMMs = %d, want 1（Absent 槽位要滤掉）: %+v", len(snap.Memory.DIMMs), snap.Memory.DIMMs)
	}
	if snap.Memory.DIMMs[0].Slot != "DIMM000 A0" {
		t.Errorf("槽位 = %q, want DeviceLocator 'DIMM000 A0'", snap.Memory.DIMMs[0].Slot)
	}
	if snap.System.Model != "Kunpeng Server Board S920 S00" {
		t.Errorf("Model = %q", snap.System.Model)
	}
	if snap.System.BMCModel != "iBMC" {
		t.Errorf("BMCModel = %q, want iBMC", snap.System.BMCModel)
	}
	if snap.Power.Redundancy != "Sparing" || snap.Power.TotalWatts != 415 {
		t.Errorf("电源 = %q/%v", snap.Power.Redundancy, snap.Power.TotalWatts)
	}
}

// dellRoutes 是标准 DMTF 布局（Storage 单数、SEL 在 Managers 下、无 Oem 视图），
// 保证为华为做的改动没有把 Dell 搞坏。
func dellRoutes() map[string]string {
	return map[string]string{
		"/redfish/v1/Systems":                   `{"Members":[{"@odata.id":"/redfish/v1/Systems/System.Embedded.1"}]}`,
		"/redfish/v1/Chassis":                   `{"Members":[{"@odata.id":"/redfish/v1/Chassis/System.Embedded.1"}]}`,
		"/redfish/v1/Managers":                  `{"Members":[{"@odata.id":"/redfish/v1/Managers/iDRAC.Embedded.1"}]}`,
		"/redfish/v1/Managers/iDRAC.Embedded.1": `{"Model":"iDRAC9","FirmwareVersion":"6.10.80.00"}`,
		"/redfish/v1/Systems/System.Embedded.1": `{
			"Status":{"Health":"Critical","State":"Enabled"},
			"Manufacturer":"Dell Inc.","Model":"PowerEdge R740","SKU":"7X8K2M3",
			"BiosVersion":"2.19.1","PowerState":"On",
			"MemorySummary":{"TotalSystemMemoryGiB":384},
			"Storage":{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Storage"},
			"Processors":{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Processors"},
			"Memory":{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Memory"},
			"Links":{"Chassis":[{"@odata.id":"/redfish/v1/Chassis/System.Embedded.1"}]}}`,
		"/redfish/v1/Systems/System.Embedded.1/Processors": `{"Members":[{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Processors/CPU.Socket.1"}]}`,
		"/redfish/v1/Systems/System.Embedded.1/Processors/CPU.Socket.1": `{
			"Name":"CPU.Socket.1","Model":"Intel Xeon Gold 6248R","ProcessorType":"CPU",
			"TotalCores":24,"TotalThreads":48,"MaxSpeedMHz":4000,"Status":{"Health":"OK","State":"Enabled"}}`,
		"/redfish/v1/Systems/System.Embedded.1/Memory":  `{"Members":[]}`,
		"/redfish/v1/Systems/System.Embedded.1/Storage": `{"Members":[{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Storage/RAID.Integrated.1-1"}]}`,
		"/redfish/v1/Systems/System.Embedded.1/Storage/RAID.Integrated.1-1": `{
			"Name":"PERC H740P Mini",
			"StorageControllers":[{"Name":"PERC H740P Mini","Model":"PERC H740P Mini",
				"FirmwareVersion":"51.16.0-4076","Status":{"Health":"OK","State":"Enabled"}}],
			"Drives":[{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Storage/Drives/Disk.Bay.0"}]}`,
		"/redfish/v1/Systems/System.Embedded.1/Storage/Drives/Disk.Bay.0": `{
			"Name":"Disk 0","Model":"MZILT3T8HBLS0D3","CapacityBytes":3840755982336,
			"MediaType":"SSD","Protocol":"SAS","Status":{"Health":"OK","State":"Enabled"},
			"PhysicalLocation":{"PartLocation":{"ServiceLabel":"Bay 0"}}}`,
		"/redfish/v1/Chassis/System.Embedded.1": `{
			"Thermal":{"@odata.id":"/redfish/v1/Chassis/System.Embedded.1/Thermal"},
			"Power":{"@odata.id":"/redfish/v1/Chassis/System.Embedded.1/Power"}}`,
		"/redfish/v1/Chassis/System.Embedded.1/Thermal": `{"Temperatures":[
			{"Name":"CPU1 Temp","ReadingCelsius":62,"Status":{"Health":"OK"},
			 "UpperThresholdCaution":85,"UpperThresholdCritical":95}],"Fans":[]}`,
		"/redfish/v1/Chassis/System.Embedded.1/Power": `{
			"PowerControl":[{"PowerConsumedWatts":612}],
			"PowerSupplies":[{"Name":"PS1 Status","PowerInputWatts":320,"Status":{"Health":"OK","State":"Enabled"}}],
			"Redundancy":[{"Mode":"N+m"}]}`,
		"/redfish/v1/Systems/System.Embedded.1/LogServices": `{"Members":[]}`,
		"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices": `{"Members":[
			{"@odata.id":"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/Lclog"},
			{"@odata.id":"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/Sel"}]}`,
		"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/Sel": `{"Entries":{"@odata.id":"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/Sel/Entries"}}`,
		"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/Sel/Entries": `{"Members":[
			{"Id":"1","Created":"2026-07-17T10:00:00+08:00","Severity":"Critical",
			 "Message":"Power supply redundancy is lost.","MessageId":"PSU0322","MessageArgs":["PS2 Status"]}]}`,
		"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/Lclog": `{"Entries":{"@odata.id":"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/Lclog/Entries"}}`,
		"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/Lclog/Entries": `{"Members":[
			{"Id":"99","Created":"2026-07-17T12:00:00+08:00","Severity":"OK","Message":"Config changed"}]}`,
		"/redfish/v1/UpdateService/FirmwareInventory": `{"Members":[]}`,
	}
}

func TestDellStillWorksAfterHuaweiChanges(t *testing.T) {
	srv := serveRoutes(t, dellRoutes())
	defer srv.Close()

	rc := newRedfishCollector(nil, "h2", "fp")
	tgt := RedfishTarget{Name: "idrac", URL: srv.URL, Username: "u", Password: "p"}
	snap := rc.collectOne(tgt)
	if snap.Error != "" {
		t.Fatalf("collectOne error = %q", snap.Error)
	}
	if snap.System.SerialNumber != "7X8K2M3" {
		t.Errorf("SerialNumber = %q, want 回落到 SKU", snap.System.SerialNumber)
	}
	if len(snap.CPUs) != 1 || snap.CPUs[0].Cores != 24 {
		t.Errorf("CPUs = %+v, want 标准 /Processors 回落路径生效", snap.CPUs)
	}
	if len(snap.Storage) != 1 || snap.Storage[0].Location != "Bay 0" {
		t.Errorf("Storage = %+v, want Disk 0 @ Bay 0", snap.Storage)
	}
	if len(snap.RAID) != 1 {
		t.Errorf("RAID = %+v, want PERC H740P", snap.RAID)
	}
	// Dell 的 Lclog 全是配置变更噪声，必须选 Sel
	rc.mu.Lock()
	pinned := rc.logPath[tgt.Name]
	rc.mu.Unlock()
	if !strings.HasSuffix(pinned, "/Sel") {
		t.Errorf("选中的日志服务 = %q, want Sel（而非 Lclog）", pinned)
	}
	if len(snap.Events) != 1 || snap.Events[0].Component != "PS2 Status" {
		t.Errorf("Events = %+v, want 归因到 PS2 Status（MessageArgs[0]）", snap.Events)
	}
}

func TestHwSeverityNorm(t *testing.T) {
	cases := map[string]string{
		"Critical": "Critical", "CRIT": "Critical", "Major": "Critical", "Fatal": "Critical",
		"Warning": "Warning", "WARN": "Warning", "Minor": "Warning",
		"OK": "OK", "Normal": "OK", "Informational": "OK",
		"": "", "Bogus": "",
	}
	for in, want := range cases {
		if got := hwSeverityNorm(in); got != want {
			t.Errorf("hwSeverityNorm(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOrDefaultAndComponentFromODataID(t *testing.T) {
	if orDefault("", "fb") != "fb" || orDefault("  ", "fb") != "fb" || orDefault("v", "fb") != "v" {
		t.Error("orDefault 行为不符")
	}
	cases := map[string]string{
		"/redfish/v1/Systems/1/Memory/DIMM.Socket.A3": "DIMM.Socket.A3",
		"/redfish/v1/Chassis/1/Drives/HDDPlaneDisk0":  "HDDPlaneDisk0",
		"/redfish/v1/Systems/1/":                      "1",
		"":                                            "",
		"NoSlash":                                     "NoSlash",
	}
	for in, want := range cases {
		if got := componentFromODataID(in); got != want {
			t.Errorf("componentFromODataID(%q) = %q, want %q", in, got, want)
		}
	}
}
