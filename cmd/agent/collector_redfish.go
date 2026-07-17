package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// redfishTLSConfig returns a tls.Config tuned for BMC/iDRAC/iLO compatibility.
// Old firmware (Dell iDRAC 7/8, HP iLO 3/4, Supermicro IPMI) often only supports
// TLS 1.0/1.1 and RSA key-exchange cipher suites that Go 1.22+ no longer offers
// by default. This config explicitly enables those legacy options so the handshake
// can succeed. BMC devices are internal-network only, so the reduced crypto
// requirements are acceptable.
func redfishTLSConfig(skipVerify bool) *tls.Config {
	// Start with all ID-based cipher suites (Go default set)
	cipherIDs := make([]uint16, 0, 32)
	for _, cs := range tls.CipherSuites() {
		cipherIDs = append(cipherIDs, cs.ID)
	}
	// Append insecure suites required by legacy BMC firmware:
	//   - RSA key exchange (TLS_RSA_WITH_AES_*_CBC_SHA)
	//   - 3DES suites
	for _, cs := range tls.InsecureCipherSuites() {
		cipherIDs = append(cipherIDs, cs.ID)
	}
	return &tls.Config{
		MinVersion:         tls.VersionTLS10, // allow TLS 1.0 for old iDRAC/iLO
		CipherSuites:       cipherIDs,
		InsecureSkipVerify: skipVerify,
	}
}

// redfishTransport creates an http.Transport configured for BMC compatibility.
// DisableKeepAlives is set because Dell iDRAC / HP iLO HTTP implementations
// send stale data on idle connections, causing Go's HTTP client to log
// "Unsolicited response received on idle HTTP channel". Each Redfish request
// is independent (30-60s apart), so connection reuse provides no benefit.
func redfishTransport(skipVerify bool) *http.Transport {
	return &http.Transport{
		TLSClientConfig:   redfishTLSConfig(skipVerify),
		DisableKeepAlives: true,
	}
}

// RedfishTarget is one BMC/iDRAC/iLO endpoint to poll (from config.json).
type RedfishTarget struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	Username      string `json:"username"`
	PasswordEnv   string `json:"password_env"`       // 密码所在环境变量名（优先，密码不落盘）
	Password      string `json:"password,omitempty"` // 直接密码（备选：当环境变量不可用时，如 systemd 服务）
	SkipTLSVerify bool   `json:"skip_tls_verify"`
	IntervalSec   int    `json:"interval_sec"`
}

// resolvePassword returns the effective password for this target.
// Priority: environment variable (password_env) > direct field (password).
// Logs diagnostics when the password appears empty.
func (t RedfishTarget) resolvePassword() string {
	pw := ""
	if t.PasswordEnv != "" {
		pw = os.Getenv(t.PasswordEnv)
		if pw == "" {
			slog.Warn("Redfish 密码环境变量为空",
				"target", t.Name, "env", t.PasswordEnv,
				"hint", "systemd 服务不继承用户环境变量，请在 .service 文件中设置 EnvironmentFile 或使用 password 字段")
		}
	}
	if pw == "" && t.Password != "" {
		pw = t.Password
	}
	if pw == "" {
		slog.Error("Redfish 密码为空，认证将失败",
			"target", t.Name,
			"password_env", t.PasswordEnv,
			"has_password_field", t.Password != "",
			"fix", "1) 设置环境变量并配置 EnvironmentFile，或 2) 在 config.json 中添加 password 字段")
	}
	return pw
}

// redfishCollector manages periodic polling of one or more Redfish endpoints.
// Each target runs in its own goroutine with an independent timer.
type redfishCollector struct {
	targets []RedfishTarget
	hostID  string
	fp      string
	httpc   *http.Client

	mu        sync.Mutex
	snapshots []shared.HardwareSnapshot
	lastFW    map[string]int64 // target_name → last firmware collect timestamp
	lastSEL   map[string]int64 // target_name → last event-log collect timestamp
	// 事件日志按 selInterval 降频采集，但每份快照都要带上——快照是整体 upsert 的，
	// 不带就等于把上一轮的事件清空，UI 上事件表会一闪一闪。
	selCache map[string][]shared.HardwareEvent
	logPath  map[string]string // target_name → 已选定的 LogService 路径（避免每轮重新发现）

	// systemPath caches the discovered Systems member @odata.id per target
	// (e.g. "/redfish/v1/Systems/System.Embedded.1" for Dell iDRAC).
	// Avoids hardcoding "/redfish/v1/Systems/1" which varies by vendor.
	sysPathMu   sync.Mutex
	systemPath  map[string]string // target_name → discovered system path
	chassisPath map[string]string // target_name → discovered chassis path
}

func newRedfishCollector(targets []RedfishTarget, hostID, fp string) *redfishCollector {
	return &redfishCollector{
		targets: targets,
		hostID:  hostID,
		fp:      fp,
		httpc: &http.Client{
			Timeout:   30 * time.Second,
			Transport: redfishTransport(false),
		},
		lastFW:      make(map[string]int64),
		lastSEL:     make(map[string]int64),
		selCache:    make(map[string][]shared.HardwareEvent),
		logPath:     make(map[string]string),
		systemPath:  make(map[string]string),
		chassisPath: make(map[string]string),
	}
}

// run starts one goroutine per target. Called from Agent.Run().
func (rc *redfishCollector) run(reporter func(shared.HardwareReport)) {
	for _, t := range rc.targets {
		go rc.pollLoop(t, reporter)
	}
}

func (rc *redfishCollector) pollLoop(t RedfishTarget, reporter func(shared.HardwareReport)) {
	interval := time.Duration(t.IntervalSec) * time.Second
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}

	slog.Info("Redfish 采集器启动", "target", t.Name, "url", t.URL, "interval", interval, "skip_tls", t.SkipTLSVerify)

	failCount := 0
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Collect immediately on start
	snap := rc.collectOne(t)
	if snap.Error != "" {
		failCount++
		slog.Warn("Redfish 采集失败", "target", t.Name, "err", snap.Error)
	} else {
		failCount = 0
		// 首次采集把各部件条数打出来：某类部件为 0 往往意味着该机型的 Redfish
		// 布局又有新花样（华为的 /Storages 就是这么暴露的），日志里一眼可见，
		// 不用等人去翻代码或上机器抓包。
		slog.Info("Redfish 采集成功", "target", t.Name, "health", snap.Health,
			"model", snap.System.Model, "cpu", len(snap.CPUs), "dimm", len(snap.Memory.DIMMs),
			"disk", len(snap.Storage), "raid", len(snap.RAID), "psu", len(snap.Power.PSUs),
			"fan", len(snap.Fans), "temp", len(snap.Temps), "event", len(snap.Events))
	}
	rc.storeAndReport(t, snap, reporter)

	for range ticker.C {
		snap := rc.collectOne(t)
		if snap.Error != "" {
			failCount++
			slog.Warn("Redfish 采集失败", "target", t.Name, "err", snap.Error, "consecutive", failCount)
			if failCount >= 3 {
				// Backoff to 5 minutes on consecutive failures
				slog.Error("Redfish 连续失败，退避 5 分钟", "target", t.Name)
				time.Sleep(5 * time.Minute)
				failCount = 0
			}
		} else {
			failCount = 0
		}
		rc.storeAndReport(t, snap, reporter)
	}
}

func (rc *redfishCollector) storeAndReport(t RedfishTarget, snap shared.HardwareSnapshot, reporter func(shared.HardwareReport)) {
	rc.mu.Lock()
	// Update or append snapshot
	found := false
	for i, s := range rc.snapshots {
		if s.TargetName == snap.TargetName && s.TargetURL == snap.TargetURL {
			rc.snapshots[i] = snap
			found = true
			break
		}
	}
	if !found {
		rc.snapshots = append(rc.snapshots, snap)
	}
	all := make([]shared.HardwareSnapshot, len(rc.snapshots))
	copy(all, rc.snapshots)
	rc.mu.Unlock()

	reporter(shared.HardwareReport{
		HostID:      rc.hostID,
		Fingerprint: rc.fp,
		Snapshots:   all,
	})
}

// discoverSystemPath queries /redfish/v1/Systems and returns the first
// member's @odata.id. This handles vendor-specific system IDs:
//   - Dell iDRAC:   /redfish/v1/Systems/System.Embedded.1
//   - HP iLO:       /redfish/v1/Systems/1
//   - Supermicro:   /redfish/v1/Systems/1
//   - Lenovo XCC:   /redfish/v1/Systems/1
func (rc *redfishCollector) discoverSystemPath(client *http.Client, t RedfishTarget) (string, error) {
	password := t.resolvePassword()
	var col struct {
		Members []struct {
			ODataID string `json:"@odata.id"`
		} `json:"Members"`
	}
	if err := rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Systems", &col); err != nil {
		return "", fmt.Errorf("discover Systems collection: %w", err)
	}
	if len(col.Members) == 0 {
		return "", fmt.Errorf("Systems collection is empty")
	}
	path := col.Members[0].ODataID
	slog.Info("Redfish System 路径已发现", "target", t.Name, "path", path)
	return path, nil
}

// getSystemPath returns the cached system path for a target, discovering
// it on first call.
func (rc *redfishCollector) getSystemPath(client *http.Client, t RedfishTarget) (string, error) {
	rc.sysPathMu.Lock()
	if p, ok := rc.systemPath[t.Name]; ok {
		rc.sysPathMu.Unlock()
		return p, nil
	}
	rc.sysPathMu.Unlock()

	p, err := rc.discoverSystemPath(client, t)
	if err != nil {
		return "", err
	}
	rc.sysPathMu.Lock()
	rc.systemPath[t.Name] = p
	rc.sysPathMu.Unlock()
	return p, nil
}

// getChassisPath returns the cached chassis path, discovering from the
// system's Links.Chassis array or the Chassis collection.
func (rc *redfishCollector) getChassisPath(client *http.Client, t RedfishTarget, sysPath string) (string, error) {
	rc.sysPathMu.Lock()
	if p, ok := rc.chassisPath[t.Name]; ok {
		rc.sysPathMu.Unlock()
		return p, nil
	}
	rc.sysPathMu.Unlock()

	password := t.resolvePassword()

	// Try from System.Links.Chassis first
	var sysLinks struct {
		Links struct {
			Chassis []struct {
				ODataID string `json:"@odata.id"`
			} `json:"Chassis"`
		} `json:"Links"`
	}
	if rc.rfGetRaw(client, t.URL, t.Username, password, sysPath, &sysLinks) == nil && len(sysLinks.Links.Chassis) > 0 {
		p := sysLinks.Links.Chassis[0].ODataID
		slog.Info("Redfish Chassis 路径已发现(via Links)", "target", t.Name, "path", p)
		rc.sysPathMu.Lock()
		rc.chassisPath[t.Name] = p
		rc.sysPathMu.Unlock()
		return p, nil
	}

	// Fallback: query Chassis collection
	var col struct {
		Members []struct {
			ODataID string `json:"@odata.id"`
		} `json:"Members"`
	}
	if err := rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Chassis", &col); err != nil {
		return "", fmt.Errorf("discover Chassis collection: %w", err)
	}
	if len(col.Members) == 0 {
		return "", fmt.Errorf("Chassis collection is empty")
	}
	p := col.Members[0].ODataID
	slog.Info("Redfish Chassis 路径已发现", "target", t.Name, "path", p)
	rc.sysPathMu.Lock()
	rc.chassisPath[t.Name] = p
	rc.sysPathMu.Unlock()
	return p, nil
}

// classifyError returns a human-readable hint for common Redfish errors.
func classifyError(err error) string {
	msg := err.Error()
	switch {
	case containsAny(msg, "handshake failure", "tls: "):
		return "（TLS 握手失败：已启用 TLS 1.0+ 兼容模式，若仍失败请检查 BMC 固件版本是否过低，或尝试升级 iDRAC/iLO 固件）"
	case containsAny(msg, "x509", "certificate"):
		return "（TLS 证书错误：请在配置中设置 skip_tls_verify=true）"
	case containsAny(msg, "connection refused", "connect: "):
		return "（连接被拒绝：请检查 BMC 地址和端口是否正确，以及防火墙是否放行）"
	case containsAny(msg, "no such host", "lookup"):
		return "（DNS 解析失败：请检查 BMC 地址是否可达）"
	case containsAny(msg, "timeout", "deadline exceeded"):
		return "（连接超时：BMC 可能不可达或网络不通）"
	case containsAny(msg, "HTTP 401", "HTTP 403"):
		return "（认证失败：请检查 username 和 password_env 环境变量是否正确）"
	default:
		return ""
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// collectOne does a full sweep of one Redfish target and returns a snapshot.
func (rc *redfishCollector) collectOne(t RedfishTarget) shared.HardwareSnapshot {
	snap := shared.HardwareSnapshot{
		TargetName: t.Name,
		TargetURL:  t.URL,
		Timestamp:  time.Now().Unix(),
	}

	password := t.resolvePassword()

	// Build per-target HTTP client with optional TLS skip
	client := rc.httpc
	if t.SkipTLSVerify {
		client = &http.Client{
			Timeout:   30 * time.Second,
			Transport: redfishTransport(true),
		}
	}

	// Discover system path (vendor-agnostic)
	sysPath, err := rc.getSystemPath(client, t)
	if err != nil {
		hint := classifyError(err)
		snap.Error = fmt.Sprintf("%v %s", err, hint)
		return snap
	}

	// Discover chassis path (vendor-agnostic)
	chassisPath, _ := rc.getChassisPath(client, t, sysPath)
	// Chassis 上取 Thermal/Power 的真实链接与物理盘列表。华为把盘挂在这里，
	// 而 Thermal/Power 各家都还算标准（拿不到链接就按标准路径拼）。
	thermalPath, powerPath, chassisDrives := rc.chassisLinks(client, t, password, chassisPath)

	// 1. System overview（含整机身份：厂商/型号/序列号/BIOS）
	var sys struct {
		Status           redfishStatus `json:"Status"`
		Manufacturer     string        `json:"Manufacturer"`
		Model            string        `json:"Model"`
		SKU              string        `json:"SKU"`
		SerialNumber     string        `json:"SerialNumber"`
		PartNumber       string        `json:"PartNumber"`
		AssetTag         string        `json:"AssetTag"`
		HostName         string        `json:"HostName"`
		BiosVersion      string        `json:"BiosVersion"`
		PowerState       string        `json:"PowerState"`
		IndicatorLED     string        `json:"IndicatorLED"`
		ProcessorSummary struct {
			Count       int    `json:"Count"`
			Model       string `json:"Model"`
			CoreCount   int    `json:"CoreCount"`
			ThreadCount int    `json:"ThreadCount"`
		} `json:"ProcessorSummary"`
		MemorySummary struct {
			TotalSystemMemoryGiB float64 `json:"TotalSystemMemoryGiB"`
		} `json:"MemorySummary"`
		// 子资源一律**跟随链接**，绝不用 sysPath+"/Storage" 这类拼接：
		// 华为 iBMC 的 Storage 属性名虽是标准的，值却指向 /Systems/1/Storages(复数)，
		// 拼接式路径在华为机器上直接 404 —— 整个存储区块因此永远是空的。
		Storage     odataRef `json:"Storage"`
		Memory      odataRef `json:"Memory"`
		Processors  odataRef `json:"Processors"`
		LogServices odataRef `json:"LogServices"`
		Oem         struct {
			// Go 的 json 解码本身大小写不敏感，Oem.Huawei / Oem.huawei 都能命中。
			Huawei struct {
				ProcessorView odataRef `json:"ProcessorView"`
				MemoryView    odataRef `json:"MemoryView"`
			} `json:"Huawei"`
		} `json:"Oem"`
	}
	if err := rc.rfGetRaw(client, t.URL, t.Username, password, sysPath, &sys); err != nil {
		hint := classifyError(err)
		snap.Error = fmt.Sprintf("Systems: %v %s", err, hint)
		return snap
	}
	snap.Health = sys.Status.Health
	snap.State = sys.Status.State
	if snap.Health == "" {
		snap.Health = "OK"
	}
	snap.Memory.TotalGB = sys.MemorySummary.TotalSystemMemoryGiB
	snap.System = shared.RedfishSystem{
		Manufacturer: strings.TrimSpace(sys.Manufacturer),
		Model:        strings.TrimSpace(sys.Model),
		SKU:          strings.TrimSpace(sys.SKU),
		SerialNumber: strings.TrimSpace(sys.SerialNumber),
		AssetTag:     strings.TrimSpace(sys.AssetTag),
		HostName:     strings.TrimSpace(sys.HostName),
		BIOSVersion:  strings.TrimSpace(sys.BiosVersion),
		PowerState:   sys.PowerState,
		IndicatorLED: sys.IndicatorLED,
	}
	// 华为 iBMC(RH2288 V3 / TaiShan 200) 常把序列号只填在 Chassis 的 SerialNumber，
	// System.SerialNumber 为空；Dell 则把 Service Tag 放在 SKU。两边都兜一下。
	if snap.System.SerialNumber == "" && snap.System.SKU != "" {
		snap.System.SerialNumber = snap.System.SKU
	}
	// BMC 自身信息（型号 iDRAC9/iBMC + 固件版本），失败不影响整体
	rc.fillManagerInfo(client, t, password, &snap)

	if sys.ProcessorSummary.Count > 0 {
		snap.CPUs = append(snap.CPUs, shared.RedfishCPU{
			Name:    "CPU Summary",
			Model:   sys.ProcessorSummary.Model,
			Cores:   sys.ProcessorSummary.CoreCount,
			Threads: sys.ProcessorSummary.ThreadCount,
			Health:  sys.Status.Health,
		})
	}

	// 2. Processors detail
	// 华为 iBMC 的 ProcessorView 一次 GET 就返回全部 CPU（含标准 schema 里没有的
	// 温度），比逐个成员 GET 便宜得多；拿不到再回落标准 /Processors。
	if !rc.collectHuaweiProcessorView(client, t, password, sys.Oem.Huawei.ProcessorView.ID, &snap) {
		rc.collectProcessors(client, t, password, orDefault(sys.Processors.ID, sysPath+"/Processors"), &snap)
	}

	// 3. Memory DIMMs
	// 同 CPU：华为 MemoryView 一次拿全，标准 /Memory 兜底。
	var mems struct {
		Members []struct {
			ODataID string `json:"@odata.id"`
		} `json:"Members"`
	}
	if !rc.collectHuaweiMemoryView(client, t, password, sys.Oem.Huawei.MemoryView.ID, &snap) &&
		rc.rfGetRaw(client, t.URL, t.Username, password, orDefault(sys.Memory.ID, sysPath+"/Memory"), &mems) == nil {
		for _, m := range mems.Members {
			var dimm struct {
				Name              string        `json:"Name"`
				CapacityMiB       float64       `json:"CapacityMiB"`
				MemoryDeviceType  string        `json:"MemoryDeviceType"`
				OperatingSpeedMhz int           `json:"OperatingSpeedMhz"`
				AllowedSpeedsMHz  []int         `json:"AllowedSpeedsMHz"`
				Status            redfishStatus `json:"Status"`
				Id                string        `json:"Id"`
				DeviceLocator     string        `json:"DeviceLocator"`
				Manufacturer      string        `json:"Manufacturer"`
				PartNumber        string        `json:"PartNumber"`
				SerialNumber      string        `json:"SerialNumber"`
				RankCount         int           `json:"RankCount"`
			}
			if rc.rfGetRaw(client, t.URL, t.Username, password, m.ODataID, &dimm) != nil {
				continue
			}
			// 空槽位也会作为成员返回（State=Absent / 容量 0）。把它们混进列表会让
			// "24 条内存" 里一半是幻影，异常计数也跟着虚高。
			if strings.EqualFold(dimm.Status.State, "Absent") || dimm.CapacityMiB <= 0 {
				continue
			}
			// DeviceLocator 才是机箱丝印上的槽位（A1/DIMM010 J10），Id 多是 "DIMM.Socket.A1"
			// 这种内部路径，运维拿去插拔对不上。
			slot := strings.TrimSpace(dimm.DeviceLocator)
			if slot == "" {
				slot = dimm.Id
			}
			speed := dimm.OperatingSpeedMhz
			if speed == 0 && len(dimm.AllowedSpeedsMHz) > 0 {
				speed = dimm.AllowedSpeedsMHz[len(dimm.AllowedSpeedsMHz)-1]
			}
			snap.Memory.DIMMs = append(snap.Memory.DIMMs, shared.MemoryDIMM{
				Name:         dimm.Name,
				CapacityGB:   dimm.CapacityMiB / 1024,
				Type:         dimm.MemoryDeviceType,
				SpeedMHz:     speed,
				Health:       dimm.Status.Health,
				Slot:         slot,
				Manufacturer: strings.TrimSpace(dimm.Manufacturer),
				PartNumber:   strings.TrimSpace(dimm.PartNumber),
				SerialNumber: strings.TrimSpace(dimm.SerialNumber),
				RankCount:    dimm.RankCount,
				State:        dimm.Status.State,
			})
		}
	}

	// 4. Storage
	// 路径**必须**取自 System.Storage 的 @odata.id：华为 iBMC(含 Kunpeng S920 S00 主板)
	// 把它指向 /Systems/{id}/Storages（复数），硬拼 "/Storage" 会 404 → 存储、RAID 卡、
	// 逻辑卷、硬盘全军覆没。链接缺失时才退回标准路径。
	var storages struct {
		Members []struct {
			ODataID string `json:"@odata.id"`
		} `json:"Members"`
	}
	// 已见过的盘 URI —— 华为的盘同时挂在 Storage 成员和 Chassis.Links.Drives 下，
	// 两边都要采（有的机型只有其中一边有），靠 URI 去重避免同一块盘出现两次。
	seenDrives := map[string]bool{}
	if rc.rfGetRaw(client, t.URL, t.Username, password, orDefault(sys.Storage.ID, sysPath+"/Storage"), &storages) == nil {
		for _, sm := range storages.Members {
			var st struct {
				Name   string `json:"Name"`
				Drives []struct {
					ODataID string `json:"@odata.id"`
				} `json:"Drives"`
				Volumes struct {
					ODataID string `json:"@odata.id"`
				} `json:"Volumes"`
				// RAID / HBA 控制器就在 Storage 成员的 StorageControllers 里。
				StorageControllers []struct {
					Name            string        `json:"Name"`
					Model           string        `json:"Model"`
					Manufacturer    string        `json:"Manufacturer"`
					FirmwareVersion string        `json:"FirmwareVersion"`
					SerialNumber    string        `json:"SerialNumber"`
					SpeedGbps       float64       `json:"SpeedGbps"`
					Status          redfishStatus `json:"Status"`
					CacheSummary    struct {
						TotalCacheSizeMiB      float64       `json:"TotalCacheSizeMiB"`
						PersistentCacheSizeMiB float64       `json:"PersistentCacheSizeMiB"`
						Status                 redfishStatus `json:"Status"`
					} `json:"CacheSummary"`
				} `json:"StorageControllers"`
			}
			if rc.rfGetRaw(client, t.URL, t.Username, password, sm.ODataID, &st) == nil {
				vols := rc.collectVolumes(client, t, password, st.Volumes.ODataID)
				for _, ctl := range st.StorageControllers {
					name := ctl.Name
					if name == "" {
						name = st.Name
					}
					snap.RAID = append(snap.RAID, shared.RedfishRAID{
						Name:            name,
						Model:           strings.TrimSpace(ctl.Model),
						Manufacturer:    strings.TrimSpace(ctl.Manufacturer),
						FirmwareVersion: strings.TrimSpace(ctl.FirmwareVersion),
						SerialNumber:    strings.TrimSpace(ctl.SerialNumber),
						SpeedGbps:       ctl.SpeedGbps,
						Health:          ctl.Status.Health,
						State:           ctl.Status.State,
						DriveCount:      len(st.Drives),
						CacheMB:         ctl.CacheSummary.TotalCacheSizeMiB,
						CacheHealth:     ctl.CacheSummary.Status.Health,
						Volumes:         vols,
					})
				}
				for _, d := range st.Drives {
					rc.collectDrive(client, t, password, d.ODataID, seenDrives, &snap)
				}
			}
		}
	}
	// 华为 iBMC 的物理盘权威列表在 Chassis.Links.Drives（/Chassis/1/Drives/HDDPlaneDisk0），
	// 不在 Storage 成员下；某些机型两边都有、某些只有 Chassis 一边。合并去重后才完整。
	for _, d := range chassisDrives {
		rc.collectDrive(client, t, password, d, seenDrives, &snap)
	}

	// 5. Thermal (temperatures + fans)
	var thermal struct {
		Temperatures []struct {
			Name                   string        `json:"Name"`
			ReadingCelsius         float64       `json:"ReadingCelsius"`
			Status                 redfishStatus `json:"Status"`
			UpperThresholdCaution  float64       `json:"UpperThresholdCaution"`
			UpperThresholdCritical float64       `json:"UpperThresholdCritical"`
		} `json:"Temperatures"`
		Fans []struct {
			Name         string        `json:"Name"`
			Reading      int           `json:"Reading"`
			ReadingUnits string        `json:"ReadingUnits"`
			Status       redfishStatus `json:"Status"`
		} `json:"Fans"`
	}
	if thermalPath != "" {
		if rc.rfGetRaw(client, t.URL, t.Username, password, thermalPath, &thermal) == nil {
			for _, t := range thermal.Temperatures {
				snap.Temps = append(snap.Temps, shared.SensorReading{
					Name:          t.Name,
					Reading:       t.ReadingCelsius,
					Unit:          "Celsius",
					Status:        t.Status.Health,
					UpperCaution:  t.UpperThresholdCaution,
					UpperCritical: t.UpperThresholdCritical,
				})
			}
			for _, f := range thermal.Fans {
				snap.Fans = append(snap.Fans, shared.FanReading{
					Name:   f.Name,
					RPM:    f.Reading,
					Health: f.Status.Health,
					Status: f.Status.State,
				})
			}
		}
	}

	// 6. Power (PSU + watts)
	var power struct {
		PowerControl []struct {
			Name               string  `json:"Name"`
			PowerConsumedWatts float64 `json:"PowerConsumedWatts"`
		} `json:"PowerControl"`
		// DMTF Redfish Power schema 的属性名是 **PowerSupplies** 与 **Redundancy**
		// （"PowerSupply" 只是类型名，不是属性名）。此前写成 PowerSupply /
		// PowerSupplyRedundancy，导致所有厂商的 PSU 一律解析不出来 → 前端电源区永不渲染。
		PowerSupplies []struct {
			Name               string        `json:"Name"`
			PowerInputWatts    float64       `json:"PowerInputWatts"`
			PowerOutputWatts   float64       `json:"PowerOutputWatts"`
			PowerCapacityWatts float64       `json:"PowerCapacityWatts"`
			LineInputVoltage   float64       `json:"LineInputVoltage"`
			PowerSupplyType    string        `json:"PowerSupplyType"`
			Model              string        `json:"Model"`
			Manufacturer       string        `json:"Manufacturer"`
			SerialNumber       string        `json:"SerialNumber"`
			FirmwareVersion    string        `json:"FirmwareVersion"`
			Status             redfishStatus `json:"Status"`
		} `json:"PowerSupplies"`
		Redundancy []struct {
			Mode string `json:"Mode"`
		} `json:"Redundancy"`
	}
	if powerPath != "" {
		if rc.rfGetRaw(client, t.URL, t.Username, password, powerPath, &power) == nil {
			if len(power.Redundancy) > 0 {
				snap.Power.Redundancy = power.Redundancy[0].Mode
			}
			for _, pc := range power.PowerControl {
				snap.Power.TotalWatts += pc.PowerConsumedWatts
			}
			for _, ps := range power.PowerSupplies {
				// 未装的电源槽位（Absent）不算一路电源，否则 "2 路电源" 里有一路
				// 永远显示灰色未知，看着像故障。
				if strings.EqualFold(ps.Status.State, "Absent") {
					continue
				}
				snap.Power.PSUs = append(snap.Power.PSUs, shared.PSUReading{
					Name:             ps.Name,
					InputWatts:       ps.PowerInputWatts,
					OutputWatts:      ps.PowerOutputWatts,
					Health:           ps.Status.Health,
					State:            ps.Status.State,
					Model:            strings.TrimSpace(ps.Model),
					Manufacturer:     strings.TrimSpace(ps.Manufacturer),
					SerialNumber:     strings.TrimSpace(ps.SerialNumber),
					FirmwareVersion:  strings.TrimSpace(ps.FirmwareVersion),
					CapacityWatts:    ps.PowerCapacityWatts,
					LineInputVoltage: ps.LineInputVoltage,
					PowerSupplyType:  ps.PowerSupplyType,
				})
			}
		}
	}

	// 7. BMC 事件日志（SEL / LC log / iBMC 事件）——唯一能回答"是哪个部件、什么时候
	// 出的问题"的数据源。整机 Health=Critical 本身不说明任何定位信息。
	snap.Events = rc.collectEvents(client, t, password, sysPath)

	// 8. Firmware (low frequency: every hour)
	now := time.Now().Unix()
	rc.mu.Lock()
	lastFW := rc.lastFW[t.Name]
	rc.mu.Unlock()
	if now-lastFW >= 3600 {
		var fw struct {
			Members []struct {
				ODataID string `json:"@odata.id"`
			} `json:"Members"`
		}
		if rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/UpdateService/FirmwareInventory", &fw) == nil {
			for _, m := range fw.Members {
				var f struct {
					Name    string `json:"Name"`
					Version string `json:"Version"`
				}
				if rc.rfGetRaw(client, t.URL, t.Username, password, m.ODataID, &f) == nil {
					snap.Firmware = append(snap.Firmware, shared.FirmwareInfo{
						Name:    f.Name,
						Version: f.Version,
					})
				}
			}
			rc.mu.Lock()
			rc.lastFW[t.Name] = now
			rc.mu.Unlock()
		}
	}

	return snap
}

// redfishStatus is the Status object every Redfish resource carries.
type redfishStatus struct {
	Health string `json:"Health"`
	State  string `json:"State"`
}

// odataRef is a Redfish link ({"@odata.id": "..."}). Sub-resource paths are
// ALWAYS taken from these, never string-concatenated: vendors point standard
// property names at non-standard paths (Huawei's System.Storage →
// "/redfish/v1/Systems/1/Storages"), and a hardcoded path just 404s.
type odataRef struct {
	ID string `json:"@odata.id"`
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// chassisLinks resolves the Thermal/Power links and the chassis' physical drive
// list. Huawei iBMC hangs drives off Chassis.Links.Drives
// (/redfish/v1/Chassis/1/Drives/HDDPlaneDisk0) rather than under the Storage
// member, so a Storage-only sweep sees no disks at all on those machines.
func (rc *redfishCollector) chassisLinks(client *http.Client, t RedfishTarget, password, chassisPath string) (thermal, power string, drives []string) {
	if chassisPath == "" {
		return "", "", nil
	}
	var ch struct {
		Thermal odataRef   `json:"Thermal"`
		Power   odataRef   `json:"Power"`
		Drives  []odataRef `json:"Drives"` // 部分固件直接放顶层
		Links   struct {
			Drives []odataRef `json:"Drives"`
		} `json:"Links"`
	}
	if rc.rfGetRaw(client, t.URL, t.Username, password, chassisPath, &ch) != nil {
		// 读不到 Chassis 也别放弃散热/电源：退回标准路径拼接
		return chassisPath + "/Thermal", chassisPath + "/Power", nil
	}
	for _, d := range append(append([]odataRef{}, ch.Links.Drives...), ch.Drives...) {
		if d.ID != "" {
			drives = append(drives, d.ID)
		}
	}
	return orDefault(ch.Thermal.ID, chassisPath+"/Thermal"),
		orDefault(ch.Power.ID, chassisPath+"/Power"), drives
}

// collectDrive fetches one physical drive and appends it. seen dedupes by URI —
// the same disk is reachable from both the Storage member and Chassis.Links.
func (rc *redfishCollector) collectDrive(client *http.Client, t RedfishTarget, password, path string, seen map[string]bool, snap *shared.HardwareSnapshot) {
	if path == "" || seen[path] {
		return
	}
	seen[path] = true

	var drv struct {
		Name               string        `json:"Name"`
		Model              string        `json:"Model"`
		Manufacturer       string        `json:"Manufacturer"`
		SerialNumber       string        `json:"SerialNumber"`
		Revision           string        `json:"Revision"`
		CapacityBytes      uint64        `json:"CapacityBytes"`
		Status             redfishStatus `json:"Status"`
		MediaType          string        `json:"MediaType"`
		Protocol           string        `json:"Protocol"`
		FailurePredicted   bool          `json:"FailurePredicted"` // SMART 预测故障
		RotationSpeedRPM   float64       `json:"RotationSpeedRPM"`
		NegotiatedSpeedGbs float64       `json:"NegotiatedSpeedGbs"`
		HotspareType       string        `json:"HotspareType"`
		PhysicalLocation   struct {
			PartLocation struct {
				ServiceLabel         string `json:"ServiceLabel"`
				LocationOrdinalValue int    `json:"LocationOrdinalValue"`
			} `json:"PartLocation"`
			Info string `json:"Info"`
		} `json:"PhysicalLocation"`
		// SSD 剩余寿命；Redfish 里 null 表示未知，用指针区分 null 与 0%。
		PredictedMediaLifeLeftPercent *float64 `json:"PredictedMediaLifeLeftPercent"`
		// 华为把槽位/健康细节塞在 Oem 里，标准字段常为空。
		Oem struct {
			Huawei struct {
				Position       string `json:"Position"`
				FirmwareStatus string `json:"FirmwareStatus"`
			} `json:"Huawei"`
		} `json:"Oem"`
	}
	if rc.rfGetRaw(client, t.URL, t.Username, password, path, &drv) != nil {
		return
	}
	if strings.EqualFold(drv.Status.State, "Absent") {
		return // 空盘位
	}

	loc := strings.TrimSpace(drv.PhysicalLocation.PartLocation.ServiceLabel)
	if loc == "" {
		loc = strings.TrimSpace(drv.PhysicalLocation.Info)
	}
	if loc == "" {
		loc = strings.TrimSpace(drv.Oem.Huawei.Position) // 华为的槽位在这
	}
	life := -1.0 // -1 = BMC 未提供（区别于真的剩 0%）
	if drv.PredictedMediaLifeLeftPercent != nil {
		life = *drv.PredictedMediaLifeLeftPercent
	}
	snap.Storage = append(snap.Storage, shared.RedfishStorage{
		Name:       drv.Name,
		Model:      strings.TrimSpace(drv.Model),
		CapacityGB: float64(drv.CapacityBytes) / (1024 * 1024 * 1024),
		Health:     drv.Status.Health,
		MediaType:  drv.MediaType,
		Protocol:   drv.Protocol,
		Status:     drv.Status.Health,
		// 此前 SMARTWarn 从未被赋值，前端却按它标红——盘的预测故障永远看不到。
		SMARTWarn:    drv.FailurePredicted,
		SerialNumber: strings.TrimSpace(drv.SerialNumber),
		Revision:     strings.TrimSpace(drv.Revision),
		Location:     loc,
		Manufacturer: strings.TrimSpace(drv.Manufacturer),
		RotationRPM:  int(drv.RotationSpeedRPM),
		LifeLeftPct:  life,
		SpeedGbps:    drv.NegotiatedSpeedGbs,
		HotspareType: drv.HotspareType,
		State:        drv.Status.State,
	})
}

// collectProcessors walks the standard /Processors collection, splitting CPUs
// from GPUs/accelerators by ProcessorType.
func (rc *redfishCollector) collectProcessors(client *http.Client, t RedfishTarget, password, path string, snap *shared.HardwareSnapshot) {
	var procs struct {
		Members []odataRef `json:"Members"`
	}
	if rc.rfGetRaw(client, t.URL, t.Username, password, path, &procs) != nil {
		return
	}
	snap.CPUs = nil // 用逐个处理器的明细替换掉 summary 占位
	for _, m := range procs.Members {
		var p struct {
			Name          string        `json:"Name"`
			Model         string        `json:"Model"`
			Manufacturer  string        `json:"Manufacturer"`
			ProcessorType string        `json:"ProcessorType"` // CPU / GPU / FPGA / Accelerator…
			TotalCores    int           `json:"TotalCores"`
			TotalThreads  int           `json:"TotalThreads"`
			MaxSpeedMHz   int           `json:"MaxSpeedMHz"`
			Status        redfishStatus `json:"Status"`
		}
		if rc.rfGetRaw(client, t.URL, t.Username, password, m.ID, &p) != nil {
			continue
		}
		if strings.EqualFold(p.Status.State, "Absent") {
			continue // 空 CPU 槽
		}
		// Processors 集合里同时挂 CPU 与 GPU/加速卡，按 ProcessorType 分流，
		// 否则 GPU 会被当成 CPU 混进 CPU 列表（且 GPU 信息完全看不到）。
		if strings.EqualFold(p.ProcessorType, "GPU") || strings.EqualFold(p.ProcessorType, "Accelerator") {
			snap.GPUs = append(snap.GPUs, shared.RedfishGPU{
				Name: p.Name, Model: p.Model, Manufacturer: p.Manufacturer,
				Health: p.Status.Health, State: p.Status.State, MaxFreqMHz: p.MaxSpeedMHz,
			})
			continue
		}
		snap.CPUs = append(snap.CPUs, shared.RedfishCPU{
			Name: p.Name, Model: p.Model, Cores: p.TotalCores, Threads: p.TotalThreads,
			Health: p.Status.Health, MaxFreqMHz: p.MaxSpeedMHz,
		})
	}
}

// collectHuaweiProcessorView reads Oem.Huawei.ProcessorView — one GET returns
// every CPU, including a Temperature the standard Processor schema has no field
// for. Returns false when the view is absent/unusable so callers fall back.
func (rc *redfishCollector) collectHuaweiProcessorView(client *http.Client, t RedfishTarget, password, path string, snap *shared.HardwareSnapshot) bool {
	if path == "" {
		return false
	}
	var v struct {
		Information []struct {
			Name          string        `json:"Name"`
			Model         string        `json:"Model"`
			Manufacturer  string        `json:"Manufacturer"`
			ProcessorType string        `json:"ProcessorType"`
			TotalCores    int           `json:"TotalCores"`
			TotalThreads  int           `json:"TotalThreads"`
			MaxSpeedMHz   int           `json:"MaxSpeedMHz"`
			FrequencyMHz  int           `json:"FrequencyMHz"`
			Temperature   float64       `json:"Temperature"`
			Status        redfishStatus `json:"Status"`
		} `json:"Information"`
	}
	if rc.rfGetRaw(client, t.URL, t.Username, password, path, &v) != nil || len(v.Information) == 0 {
		return false
	}
	snap.CPUs = nil
	for _, p := range v.Information {
		if strings.EqualFold(p.Status.State, "Absent") {
			continue
		}
		if strings.EqualFold(p.ProcessorType, "GPU") || strings.EqualFold(p.ProcessorType, "Accelerator") {
			snap.GPUs = append(snap.GPUs, shared.RedfishGPU{
				Name: p.Name, Model: p.Model, Manufacturer: p.Manufacturer,
				Health: p.Status.Health, State: p.Status.State, MaxFreqMHz: p.MaxSpeedMHz,
			})
			continue
		}
		freq := p.MaxSpeedMHz
		if freq == 0 {
			freq = p.FrequencyMHz
		}
		snap.CPUs = append(snap.CPUs, shared.RedfishCPU{
			Name: p.Name, Model: strings.TrimSpace(p.Model), Cores: p.TotalCores,
			Threads: p.TotalThreads, Health: p.Status.Health, MaxFreqMHz: freq,
			TempC: p.Temperature,
		})
	}
	return len(snap.CPUs) > 0 || len(snap.GPUs) > 0
}

// collectHuaweiMemoryView reads Oem.Huawei.MemoryView — one GET for all DIMMs.
func (rc *redfishCollector) collectHuaweiMemoryView(client *http.Client, t RedfishTarget, password, path string, snap *shared.HardwareSnapshot) bool {
	if path == "" {
		return false
	}
	var v struct {
		Information []struct {
			Name              string        `json:"Name"`
			CapacityMiB       float64       `json:"CapacityMiB"`
			MemoryDeviceType  string        `json:"MemoryDeviceType"`
			OperatingSpeedMhz int           `json:"OperatingSpeedMhz"`
			Manufacturer      string        `json:"Manufacturer"`
			PartNumber        string        `json:"PartNumber"`
			SerialNumber      string        `json:"SerialNumber"`
			RankCount         int           `json:"RankCount"`
			DeviceLocator     string        `json:"DeviceLocator"`
			Status            redfishStatus `json:"Status"`
		} `json:"Information"`
	}
	if rc.rfGetRaw(client, t.URL, t.Username, password, path, &v) != nil || len(v.Information) == 0 {
		return false
	}
	n := 0
	for _, d := range v.Information {
		// 华为同样把未安装的槽位一并返回，必须滤掉，否则"幻影内存条"。
		if strings.EqualFold(d.Status.State, "Absent") || d.CapacityMiB <= 0 {
			continue
		}
		snap.Memory.DIMMs = append(snap.Memory.DIMMs, shared.MemoryDIMM{
			Name:         d.Name,
			CapacityGB:   d.CapacityMiB / 1024,
			Type:         d.MemoryDeviceType,
			SpeedMHz:     d.OperatingSpeedMhz,
			Health:       d.Status.Health,
			Slot:         orDefault(strings.TrimSpace(d.DeviceLocator), d.Name),
			Manufacturer: strings.TrimSpace(d.Manufacturer),
			PartNumber:   strings.TrimSpace(d.PartNumber),
			SerialNumber: strings.TrimSpace(d.SerialNumber),
			RankCount:    d.RankCount,
			State:        d.Status.State,
		})
		n++
	}
	return n > 0
}

// hwEventCap bounds how many BMC log entries ride along in each snapshot.
// A Dell SEL holds ~500 entries and the LC log thousands; shipping them all on
// every poll would bloat the report and the JSONB row for no operational gain.
const hwEventCap = 40

// selInterval throttles event-log polling. Entries only appear on real faults,
// while a full SEL fetch is one of the heaviest Redfish calls there is (old
// iDRAC8 / RH2288 V3 firmware can take seconds) — polling it every 30s would
// tax the BMC for nothing.
const selInterval = 300

// fillManagerInfo reads the BMC's own identity (iDRAC9 / iBMC + firmware
// version) from the Managers collection. Best-effort: a BMC that doesn't expose
// it just leaves the fields blank.
func (rc *redfishCollector) fillManagerInfo(client *http.Client, t RedfishTarget, password string, snap *shared.HardwareSnapshot) {
	var col struct {
		Members []struct {
			ODataID string `json:"@odata.id"`
		} `json:"Members"`
	}
	if rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Managers", &col) != nil || len(col.Members) == 0 {
		return
	}
	var mgr struct {
		Model           string `json:"Model"`
		FirmwareVersion string `json:"FirmwareVersion"`
	}
	if rc.rfGetRaw(client, t.URL, t.Username, password, col.Members[0].ODataID, &mgr) != nil {
		return
	}
	snap.System.BMCModel = strings.TrimSpace(mgr.Model)
	snap.System.BMCFirmware = strings.TrimSpace(mgr.FirmwareVersion)
}

// collectVolumes reads logical RAID volumes for one Storage member.
func (rc *redfishCollector) collectVolumes(client *http.Client, t RedfishTarget, password, volPath string) []shared.RedfishVolume {
	if volPath == "" {
		return nil
	}
	var col struct {
		Members []struct {
			ODataID string `json:"@odata.id"`
		} `json:"Members"`
	}
	if rc.rfGetRaw(client, t.URL, t.Username, password, volPath, &col) != nil {
		return nil
	}
	var out []shared.RedfishVolume
	for _, m := range col.Members {
		var v struct {
			Name          string `json:"Name"`
			RAIDType      string `json:"RAIDType"`
			VolumeType    string `json:"VolumeType"` // 老固件（iDRAC8/RH2288 V3）只有这个
			CapacityBytes uint64 `json:"CapacityBytes"`
			Status        struct {
				Health string `json:"Health"`
				State  string `json:"State"`
			} `json:"Status"`
		}
		if rc.rfGetRaw(client, t.URL, t.Username, password, m.ODataID, &v) != nil {
			continue
		}
		rt := v.RAIDType
		if rt == "" {
			rt = v.VolumeType
		}
		out = append(out, shared.RedfishVolume{
			Name:       v.Name,
			RAIDType:   rt,
			CapacityGB: float64(v.CapacityBytes) / (1024 * 1024 * 1024),
			Health:     v.Status.Health,
			State:      v.Status.State,
		})
	}
	return out
}

// logServicePaths returns candidate LogService endpoints in priority order,
// discovered rather than hardcoded because vendors differ sharply:
//   - Dell iDRAC7/8/9: Managers/iDRAC.Embedded.1/LogServices/Sel (硬件故障)
//     外加 /Lclog（几千条配置变更噪声，不是我们要的）
//   - Huawei iBMC (RH2288 V3 / TaiShan / Kunpeng S920 S00):
//     硬件事件在 **Systems/{id}/LogServices/Log1**；
//     Managers/1/LogServices 下的 OperateLog / RunLog / SecurityLog 是
//     BMC 的操作、运行、安全日志 —— 跟硬件故障无关，选中它们等于答非所问。
//     部分固件的 Manager 甚至没有 LogServices 属性。
func (rc *redfishCollector) logServicePaths(client *http.Client, t RedfishTarget, password, sysPath string) []string {
	var out []string
	seen := map[string]bool{}

	collect := func(base string) {
		if base == "" {
			return
		}
		var col struct {
			Members []odataRef `json:"Members"`
		}
		if rc.rfGetRaw(client, t.URL, t.Username, password, base, &col) != nil {
			return
		}
		for _, m := range col.Members {
			if m.ID != "" && !seen[m.ID] {
				seen[m.ID] = true
				out = append(out, m.ID)
			}
		}
	}

	// 先 Systems（华为硬件事件在这），再 Managers（Dell 的 SEL 在这）
	if sysPath != "" {
		collect(sysPath + "/LogServices")
	}
	var mgrs struct {
		Members []odataRef `json:"Members"`
	}
	if rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Managers", &mgrs) == nil {
		for _, m := range mgrs.Members {
			collect(m.ID + "/LogServices")
		}
	}

	// 排序即取舍：数字越小越像"硬件故障日志"。
	rank := func(p string) int {
		lp := strings.ToLower(strings.TrimRight(p, "/"))
		seg := lp
		if i := strings.LastIndex(lp, "/"); i >= 0 {
			seg = lp[i+1:] // 只看最后一段，避免父路径里的字样误伤
		}
		switch {
		// BMC 自身的操作/运行/安全日志：明确排到最后，宁可没有也不要错的。
		case strings.Contains(seg, "operate"), strings.Contains(seg, "runlog"),
			strings.Contains(seg, "security"), strings.Contains(seg, "audit"):
			return 9
		case strings.Contains(seg, "lclog"), strings.Contains(seg, "lifecycle"):
			return 8 // Dell LC 日志：噪声大，仅作兜底
		case strings.Contains(seg, "sel"):
			return 0 // Dell/通用 SEL
		case strings.HasPrefix(seg, "log"):
			return 1 // 华为 Log1 / Log
		case strings.Contains(seg, "event"):
			return 2
		}
		return 5
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}

// collectEvents pulls the most recent BMC log entries and resolves each one to
// the component that triggered it.
func (rc *redfishCollector) collectEvents(client *http.Client, t RedfishTarget, password, sysPath string) []shared.HardwareEvent {
	now := time.Now().Unix()
	rc.mu.Lock()
	last, cached := rc.lastSEL[t.Name], rc.selCache[t.Name]
	pinned := rc.logPath[t.Name]
	rc.mu.Unlock()
	if now-last < selInterval {
		return cached // 未到采集周期：沿用上一轮结果，避免快照 upsert 把事件抹掉
	}

	paths := []string{}
	if pinned != "" {
		paths = append(paths, pinned) // 已确认可用的日志服务，直接用
	} else {
		paths = rc.logServicePaths(client, t, password, sysPath)
	}
	for _, p := range paths {
		// Entries 的地址同样取自 LogService 自身的链接，不做 p+"/Entries" 假设。
		var svc struct {
			Entries odataRef `json:"Entries"`
			Name    string   `json:"Name"`
		}
		entriesPath := p + "/Entries"
		if rc.rfGetRaw(client, t.URL, t.Username, password, p, &svc) == nil && svc.Entries.ID != "" {
			entriesPath = svc.Entries.ID
		}

		// $top 让 BMC 只回最近的一批；不支持的固件会忽略甚至拒绝该参数，故带回退。
		var col struct {
			Members []redfishLogEntry `json:"Members"`
		}
		sep := "?"
		if strings.Contains(entriesPath, "?") {
			sep = "&"
		}
		if rc.rfGetRaw(client, t.URL, t.Username, password, entriesPath+sep+"$top=100", &col) != nil {
			if rc.rfGetRaw(client, t.URL, t.Username, password, entriesPath, &col) != nil {
				continue
			}
		}
		if len(col.Members) == 0 {
			continue
		}
		out := make([]shared.HardwareEvent, 0, len(col.Members))
		for _, m := range col.Members {
			// 部分 iBMC 固件的集合里只有 {"@odata.id": ...} 空壳，正文要再取一次。
			// 不补这一步，事件表就是一排空白行。
			if m.Message == "" && m.Created == "" && m.ODataID != "" {
				var full redfishLogEntry
				if rc.rfGetRaw(client, t.URL, t.Username, password, m.ODataID, &full) == nil {
					m = full
				}
			}
			out = append(out, m.toEvent())
		}
		// BMC 返回顺序不一（Dell 由旧到新，部分 iBMC 相反）。统一按时间倒序，
		// 再截断，保证留下的是**最近** N 条而不是最老的 N 条。
		sort.SliceStable(out, func(i, j int) bool { return out[i].Created > out[j].Created })
		if len(out) > hwEventCap {
			out = out[:hwEventCap]
		}
		rc.mu.Lock()
		rc.lastSEL[t.Name], rc.selCache[t.Name], rc.logPath[t.Name] = now, out, p
		rc.mu.Unlock()
		slog.Debug("BMC 事件日志已采集", "target", t.Name, "path", p, "entries", len(out))
		return out
	}
	// 一条日志服务都读不到（老固件/权限不足）：记下时间避免每轮重试整套发现流程。
	rc.mu.Lock()
	rc.lastSEL[t.Name] = now
	rc.mu.Unlock()
	return cached
}

// redfishLogEntry is one LogEntry as returned by any vendor's log service.
type redfishLogEntry struct {
	ODataID      string   `json:"@odata.id"`
	Id           string   `json:"Id"`
	Name         string   `json:"Name"`
	Created      string   `json:"Created"`
	Severity     string   `json:"Severity"`
	Message      string   `json:"Message"`
	MessageId    string   `json:"MessageId"`
	MessageArgs  []string `json:"MessageArgs"`
	EntryType    string   `json:"EntryType"`
	SensorType   string   `json:"SensorType"`
	SensorNumber *int     `json:"SensorNumber"`
	Resolved     bool     `json:"Resolved"`
	Links        struct {
		OriginOfCondition odataRef `json:"OriginOfCondition"`
	} `json:"Links"`
	// 华为 iBMC 的归因与级别都在 Oem 里；Go 的 json 解码大小写不敏感，
	// 所以 Oem.Huawei / Oem.huawei（两种写法固件里都出现过）都能命中。
	Oem struct {
		Huawei struct {
			EventSubject string `json:"EventSubject"`
			Level        string `json:"Level"`
		} `json:"Huawei"`
	} `json:"Oem"`
}

// hwSeverityNorm maps a vendor severity onto Redfish's OK/Warning/Critical.
// 华为 iBMC 的 Oem.Huawei.Level 用的是 Normal/Minor/Major/WARN/CRIT 这类词，
// 直接透传会让前端按未知级别渲染成灰色"未知"，等于丢掉了告警。
func hwSeverityNorm(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "OK", "NORMAL", "INFORMATIONAL", "INFO":
		return "OK"
	case "WARNING", "WARN", "MINOR":
		return "Warning"
	case "CRITICAL", "CRIT", "MAJOR", "FATAL", "ERROR":
		return "Critical"
	}
	return ""
}

func (m redfishLogEntry) toEvent() shared.HardwareEvent {
	comp := strings.TrimSpace(m.Oem.Huawei.EventSubject)
	if comp == "" {
		comp = componentFromODataID(m.Links.OriginOfCondition.ID)
	}
	if comp == "" && len(m.MessageArgs) > 0 {
		// Dell SEL 的 MessageArgs[0] 常就是部件名（"PSU 2" / "DIMM_A3"）。
		comp = strings.TrimSpace(m.MessageArgs[0])
	}
	if comp == "" && m.SensorType != "" {
		comp = m.SensorType
		if m.SensorNumber != nil {
			comp = fmt.Sprintf("%s #%d", m.SensorType, *m.SensorNumber)
		}
	}
	// 华为在 Run Log 一类条目上 Severity 不可靠，真实级别只在 Oem.Huawei.Level。
	sev := hwSeverityNorm(m.Severity)
	if sev == "" {
		sev = hwSeverityNorm(m.Oem.Huawei.Level)
	}
	return shared.HardwareEvent{
		ID:         m.Id,
		Created:    m.Created,
		Severity:   sev,
		Message:    strings.TrimSpace(m.Message),
		MessageID:  m.MessageId,
		Component:  comp,
		SensorType: m.SensorType,
		Resolved:   m.Resolved,
	}
}

// componentFromODataID turns a Redfish resource path into a human-readable part
// name: ".../Systems/System.Embedded.1/Memory/DIMM.Socket.A3" → "DIMM.Socket.A3".
func componentFromODataID(id string) string {
	id = strings.TrimSpace(strings.TrimRight(id, "/"))
	if id == "" {
		return ""
	}
	i := strings.LastIndex(id, "/")
	if i < 0 || i == len(id)-1 {
		return id
	}
	return id[i+1:]
}

// rfGet fetches a Redfish endpoint relative to the target base URL.
func (rc *redfishCollector) rfGet(client *http.Client, base, user, pass, path string, dst any) error {
	return rc.rfGetRaw(client, base, user, pass, path, dst)
}

// rfGetRaw fetches an arbitrary Redfish path (may be @odata.id from collection members).
func (rc *redfishCollector) rfGetRaw(client *http.Client, base, user, pass, path string, dst any) error {
	url := base
	if len(path) > 0 && path[0] == '/' {
		url = base + path
	} else {
		// path is an @odata.id, already absolute on the BMC
		url = base + path
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
