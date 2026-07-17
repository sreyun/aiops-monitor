package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"aiops-monitor/shared"
)

// 用假 DeviceManager 覆盖 OceanStor 协议。这个采集器是对着文档和开源实现写的，
// 没有这层测试，任何字段/枚举/单位错误都要等上真机才暴露。

type fakeOceanStor struct {
	logins    int32
	sawToken  atomic.Bool
	sawCookie atomic.Bool
	// 令牌失效一次，用于验证自动重登
	expireOnce atomic.Bool
}

func (f *fakeOceanStor) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/sessions") && r.Method == "POST" {
			atomic.AddInt32(&f.logins, 1)
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "s3ss10n", Path: "/"})
			_, _ = w.Write([]byte(`{"data":{"deviceid":"dev123","iBaseToken":"tok456"},"error":{"code":0,"description":"0"}}`))
			return
		}

		// 真机要求 iBaseToken 头**和** cookie 同时存在，缺一被拒。
		if r.Header.Get("iBaseToken") != "" {
			f.sawToken.Store(true)
		}
		if c, err := r.Cookie("session"); err == nil && c.Value == "s3ss10n" {
			f.sawCookie.Store(true)
		}
		if r.Header.Get("iBaseToken") == "" {
			_, _ = w.Write([]byte(`{"error":{"code":-401,"description":"unauthorized"}}`))
			return
		}
		if f.expireOnce.CompareAndSwap(true, false) {
			_, _ = w.Write([]byte(`{"error":{"code":-401,"description":"token expired"}}`))
			return
		}

		body, ok := oceanStorRoutes()[strings.TrimPrefix(r.URL.Path, "/deviceManager/rest/dev123/")]
		if !ok {
			_, _ = w.Write([]byte(`{"data":[],"error":{"code":0,"description":"0"}}`))
			return
		}
		_, _ = w.Write([]byte(body))
	})
}

// 贴近真机：字段全是**字符串**，容量以扇区计，DISKTYPE/HEALTHSTATUS 是数字编码。
func oceanStorRoutes() map[string]string {
	return map[string]string{
		"system/": `{"data":{"ID":"210235','","NAME":"OceanStor-5500","PRODUCTMODESTRING":"OceanStor 5500 V5",
			"PRODUCTVERSION":"V500R007C60","HEALTHSTATUS":"1","RUNNINGSTATUS":"1","SN":"2102351NPQ10J8000012"},
			"error":{"code":0,"description":"0"}}`,
		"enclosure": `{"data":[
			{"ID":"0","NAME":"CTE0","MODEL":"OceanStor 5500 V5 Controller Enclosure","SERIALNUM":"021ABC",
			 "LOCATION":"CTE0","LOGICTYPE":"0","HEALTHSTATUS":"1","RUNNINGSTATUS":"1","TEMPERATURE":"31"},
			{"ID":"1","NAME":"DAE010","MODEL":"OceanStor Disk Enclosure","SERIALNUM":"021DEF",
			 "LOCATION":"DAE010","LOGICTYPE":"1","HEALTHSTATUS":"2","RUNNINGSTATUS":"28","TEMPERATURE":"47"}],
			"error":{"code":0,"description":"0"}}`,
		// CAPACITY 是扇区数：7025387520 * 512B ≈ 3352 GB（当字节处理会算成 6.5GB）
		"disk": `{"data":[
			{"ID":"0","MODEL":"HUSMM1640ASS200","SERIALNUMBER":"0QV1ABCD","LOCATION":"CTE0.0",
			 "CAPACITY":"7025387520","SECTORSIZE":"512","HEALTHSTATUS":"1","RUNNINGSTATUS":"27",
			 "DISKTYPE":"2","FIRMWAREVER":"K7B0","MANUFACTURER":"HUAWEI","SPEEDRPM":"0","REMAINLIFE":"97"},
			{"ID":"1","MODEL":"ST4000NM0025","SERIALNUMBER":"ZC13WXYZ","LOCATION":"DAE010.3",
			 "CAPACITY":"7814037168","SECTORSIZE":"512","HEALTHSTATUS":"3","RUNNINGSTATUS":"27",
			 "DISKTYPE":"3","FIRMWAREVER":"TN04","MANUFACTURER":"SEAGATE","SPEEDRPM":"7200"}],
			"error":{"code":0,"description":"0"}}`,
		"controller": `{"data":[
			{"ID":"0A","NAME":"CTE0.A","LOCATION":"CTE0.A","HEALTHSTATUS":"1","RUNNINGSTATUS":"27",
			 "SOFTVER":"V500R007C60","MEMORYSIZE":"65536","MODEL":"ARM"}],
			"error":{"code":0,"description":"0"}}`,
		"power": `{"data":[
			{"ID":"CTE0.PSU0","LOCATION":"CTE0.PSU0","HEALTHSTATUS":"1","RUNNINGSTATUS":"1","MODEL":"PAC900S12-B"},
			{"ID":"CTE0.PSU1","LOCATION":"CTE0.PSU1","HEALTHSTATUS":"11","RUNNINGSTATUS":"1","MODEL":"PAC900S12-B"}],
			"error":{"code":0,"description":"0"}}`,
		"fan": `{"data":[
			{"ID":"CTE0.FAN0","LOCATION":"CTE0.FAN0","HEALTHSTATUS":"1","RUNNINGSTATUS":"1","ROTATIONALSPEED":"8200"}],
			"error":{"code":0,"description":"0"}}`,
		// level 是**数字**而非字符串（与其它资源相反）
		"alarm/currentalarm": `{"data":[
			{"sequence":1024,"eventID":"0xF00D0001","level":5,"startTime":1784000000,
			 "description":"Disk enclosure DAE010 is offline.","location":"DAE010","name":"Enclosure offline"},
			{"sequence":1025,"eventID":"0xF00D0002","level":3,"startTime":1783900000,
			 "description":"Disk in slot DAE010.3 is about to fail.","location":"DAE010.3","name":"Disk pre-fail"}],
			"error":{"code":0,"description":"0"}}`,
	}
}

func newFakeOS(t *testing.T) (*fakeOceanStor, *httptest.Server) {
	t.Helper()
	f := &fakeOceanStor{}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return f, srv
}

func TestOceanStorAuthUsesTokenAndCookie(t *testing.T) {
	f, srv := newFakeOS(t)
	oc := newOceanStorCollector(nil, "h1", "fp")
	snap := oc.collectOne(OceanStorTarget{Name: "os1", URL: srv.URL, Username: "u", Password: "p"})

	if snap.Error != "" {
		t.Fatalf("collectOne error = %q", snap.Error)
	}
	if !f.sawToken.Load() {
		t.Error("请求未携带 iBaseToken 头")
	}
	// 只带 token 不带 cookie 会被真机拒绝，这条最容易漏
	if !f.sawCookie.Load() {
		t.Error("请求未携带 session cookie —— 真机会拒绝")
	}
	if n := atomic.LoadInt32(&f.logins); n != 1 {
		t.Errorf("登录次数 = %d, want 1（会话应复用，不能每个资源登一次）", n)
	}
}

func TestOceanStorReloginOnTokenExpiry(t *testing.T) {
	f, srv := newFakeOS(t)
	oc := newOceanStorCollector(nil, "h1", "fp")
	// 先建立会话
	_ = oc.collectOne(OceanStorTarget{Name: "os1", URL: srv.URL, Username: "u", Password: "p"})
	before := atomic.LoadInt32(&f.logins)

	f.expireOnce.Store(true) // 下一个请求返回 -401
	snap := oc.collectOne(OceanStorTarget{Name: "os1", URL: srv.URL, Username: "u", Password: "p"})
	if snap.Error != "" {
		t.Fatalf("令牌过期后应自动重登并成功，实际 error = %q", snap.Error)
	}
	if atomic.LoadInt32(&f.logins) <= before {
		t.Error("令牌过期后未重新登录")
	}
}

func TestOceanStorEnclosureAndDisk(t *testing.T) {
	_, srv := newFakeOS(t)
	oc := newOceanStorCollector(nil, "h1", "fp")
	snap := oc.collectOne(OceanStorTarget{Name: "os1", URL: srv.URL, Username: "u", Password: "p"})

	if snap.System.Model != "OceanStor 5500 V5" || snap.System.Manufacturer != "Huawei" {
		t.Errorf("identity = %+v", snap.System)
	}
	if snap.System.SerialNumber != "2102351NPQ10J8000012" {
		t.Errorf("SN = %q", snap.System.SerialNumber)
	}

	// 磁盘框：这是本次要解决的主角
	if len(snap.Enclosures) != 2 {
		t.Fatalf("Enclosures = %d, want 2", len(snap.Enclosures))
	}
	dae := snap.Enclosures[1]
	if dae.Name != "DAE010" || dae.Health != "Critical" || dae.State != "Offline" {
		t.Errorf("DAE010 = %+v, want Critical/Offline", dae)
	}
	if dae.TemperatureC != 47 {
		t.Errorf("DAE010 温度 = %v, want 47", dae.TemperatureC)
	}
	// 磁盘框温度必须进 Temps，否则不进温度曲线与最高温 KPI
	if len(snap.Temps) != 2 {
		t.Errorf("Temps = %d, want 2（每个框一个）", len(snap.Temps))
	}

	// 容量：扇区数 × 扇区大小，绝不能当字节
	if len(snap.Storage) != 2 {
		t.Fatalf("Storage = %d, want 2", len(snap.Storage))
	}
	ssd := snap.Storage[0]
	// 7025387520 扇区 × 512B ÷ 1024³ = 3349 GiB。若把 CAPACITY 当字节，只有 6.5GB。
	if got := int(ssd.CapacityGB); got != 3349 {
		t.Errorf("SSD 容量 = %dGB, want 3349GB（CAPACITY 是扇区数不是字节）", got)
	}
	if ssd.MediaType != "SSD" {
		t.Errorf("DISKTYPE=2 应映射为 SSD, got %q", ssd.MediaType)
	}
	if ssd.LifeLeftPct != 97 {
		t.Errorf("剩余寿命 = %v, want 97", ssd.LifeLeftPct)
	}
	hdd := snap.Storage[1]
	if hdd.MediaType != "NL-SAS" {
		t.Errorf("DISKTYPE=3 应映射为 NL-SAS, got %q", hdd.MediaType)
	}
	// HEALTHSTATUS=3 (Pre-fail) → Warning，不能升级成 Critical
	if hdd.Health != "Warning" {
		t.Errorf("Pre-fail 盘 health = %q, want Warning", hdd.Health)
	}
	// 阵列不给寿命的盘要显示 -1（前端渲染 "-"），不能是 0%
	if hdd.LifeLeftPct != -1 {
		t.Errorf("未提供寿命时 LifeLeftPct = %v, want -1", hdd.LifeLeftPct)
	}
}

func TestOceanStorPowerFanController(t *testing.T) {
	_, srv := newFakeOS(t)
	oc := newOceanStorCollector(nil, "h1", "fp")
	snap := oc.collectOne(OceanStorTarget{Name: "os1", URL: srv.URL, Username: "u", Password: "p"})

	if len(snap.Power.PSUs) != 2 {
		t.Fatalf("PSUs = %d, want 2", len(snap.Power.PSUs))
	}
	// HEALTHSTATUS=11 (No input) → Warning
	if snap.Power.PSUs[1].Health != "Warning" {
		t.Errorf("No input 电源 health = %q, want Warning", snap.Power.PSUs[1].Health)
	}
	if len(snap.Fans) != 1 || snap.Fans[0].RPM != 8200 {
		t.Errorf("Fans = %+v, want 8200 RPM", snap.Fans)
	}
	if len(snap.RAID) != 1 || snap.RAID[0].FirmwareVersion != "V500R007C60" {
		t.Errorf("控制器 = %+v", snap.RAID)
	}
}

func TestOceanStorAlarmsAttributeToComponent(t *testing.T) {
	_, srv := newFakeOS(t)
	oc := newOceanStorCollector(nil, "h1", "fp")
	snap := oc.collectOne(OceanStorTarget{Name: "os1", URL: srv.URL, Username: "u", Password: "p"})

	if len(snap.Events) != 2 {
		t.Fatalf("Events = %d, want 2", len(snap.Events))
	}
	// 最新的在前
	e := snap.Events[0]
	if e.Component != "DAE010" {
		t.Errorf("Events[0].Component = %q, want DAE010（告警必须能定位到部件）", e.Component)
	}
	if e.Severity != "Critical" {
		t.Errorf("level=5 → %q, want Critical", e.Severity)
	}
	if !strings.Contains(e.Message, "offline") {
		t.Errorf("Events[0].Message = %q", e.Message)
	}
	if snap.Events[1].Severity != "Warning" {
		t.Errorf("level=3 → %q, want Warning", snap.Events[1].Severity)
	}
	if snap.Events[1].Component != "DAE010.3" {
		t.Errorf("Events[1].Component = %q, want DAE010.3", snap.Events[1].Component)
	}
}

func TestOceanStorEnumMaps(t *testing.T) {
	// 取自华为官方 valuemap；映射错会直接产生假告警
	health := map[string]string{
		"1": "OK", "8": "OK",
		"2": "Critical", "4": "Critical",
		"3": "Warning", "5": "Warning", "6": "Warning", "7": "Warning",
		"9": "Warning", "11": "Warning", "12": "Warning", "13": "Warning",
		"0": "", "10": "", "14": "", "15": "", "99": "",
	}
	for code, want := range health {
		if got := osHealth(code); got != want {
			t.Errorf("osHealth(%q) = %q, want %q", code, got, want)
		}
	}
	if osRunning("27") != "Online" || osRunning("28") != "Offline" || osRunning("16") != "Reconstruction" {
		t.Error("osRunning 映射错误")
	}
	if osRunning("999") != "" {
		t.Error("未知 RUNNINGSTATUS 应返回空串而非瞎猜")
	}
	if osAlarmSeverity(5) != "Critical" || osAlarmSeverity(4) != "Critical" ||
		osAlarmSeverity(3) != "Warning" || osAlarmSeverity(2) != "OK" {
		t.Error("告警级别映射错误（3=warning 4=major 5=critical）")
	}
}

// 快照要能完整 JSON 往返：server 整份存 JSONB，前端按 json tag 读。
func TestOceanStorSnapshotJSON(t *testing.T) {
	_, srv := newFakeOS(t)
	oc := newOceanStorCollector(nil, "h1", "fp")
	snap := oc.collectOne(OceanStorTarget{Name: "os1", URL: srv.URL, Username: "u", Password: "p"})

	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	encs, ok := m["enclosures"].([]any)
	if !ok || len(encs) == 0 {
		t.Fatal(`快照缺少 "enclosures" 数组 —— 前端磁盘框区块会空`)
	}
	e0 := encs[0].(map[string]any)
	for _, k := range []string{"name", "health", "temperature_c"} {
		if _, ok := e0[k]; !ok {
			t.Errorf("enclosures[].%s 缺失", k)
		}
	}
}

// 两个采集器同时上报时必须合并：服务端 hardwareStore.put 是整体替换，
// 各报各的会让 Redfish 与 OceanStor 每轮互相覆盖，告警反复 fire/resolve。
func TestHardwareAggregatorMergesCollectors(t *testing.T) {
	var last shared.HardwareReport
	agg := newHardwareAggregator("h1", "fp", func(r shared.HardwareReport) { last = r })

	// 模拟 Redfish 采集器（两个 BMC）与 OceanStor 采集器（一台阵列）各自上报
	agg.submit(shared.HardwareReport{Snapshots: []shared.HardwareSnapshot{
		{TargetName: "idrac-01"}, {TargetName: "idrac-02"},
	}})
	agg.submit(shared.HardwareReport{Snapshots: []shared.HardwareSnapshot{{TargetName: "oceanstor-01"}}})

	if len(last.Snapshots) != 3 {
		t.Fatalf("合并后 %d 个快照, want 3（两边不能互相覆盖）", len(last.Snapshots))
	}
	names := []string{last.Snapshots[0].TargetName, last.Snapshots[1].TargetName, last.Snapshots[2].TargetName}
	want := []string{"idrac-01", "idrac-02", "oceanstor-01"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("上报内容/顺序 = %v, want %v", names, want)
			break
		}
	}
	if last.HostID != "h1" || last.Fingerprint != "fp" {
		t.Errorf("HostID/Fingerprint 丢失: %q/%q", last.HostID, last.Fingerprint)
	}

	// 同名 target 再次上报应更新而非追加
	agg.submit(shared.HardwareReport{Snapshots: []shared.HardwareSnapshot{{TargetName: "idrac-01", Health: "Critical"}}})
	if len(last.Snapshots) != 3 {
		t.Errorf("同名 target 重报后 %d 个快照, want 3（应更新不应追加）", len(last.Snapshots))
	}
	if last.Snapshots[0].Health != "Critical" {
		t.Errorf("同名 target 未被更新: %+v", last.Snapshots[0])
	}
}
