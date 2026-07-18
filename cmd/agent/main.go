package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"aiops-monitor/shared"
)

// ServerConfig represents one backend server target for multi-server push.
// Each entry has its own URL and optional install token; the agent reports
// to all configured servers concurrently (collect once, broadcast all).
type ServerConfig struct {
	Server string `json:"server"`
	Token  string `json:"token,omitempty"`
}

type config struct {
	Server         string         `json:"server"`               // legacy single-server field
	Servers        []ServerConfig `json:"servers,omitempty"`     // multi-server: when non-empty, takes precedence over Server+Token
	ReportInterval int            `json:"report_interval"`
	PluginInterval int            `json:"plugin_interval"`
	DiskPath       string         `json:"disk_path"`
	PluginsDir     string         `json:"plugins_dir"`
	Python         string         `json:"python"`
	StateFile      string         `json:"state_file"`
	Category       string         `json:"category"`
	Token          string         `json:"token"`               // legacy single-server token
	Relay          bool           `json:"relay"`               // gateway relay mode: proxy all requests to --server
	Listen         string         `json:"listen,omitempty"`     // relay listen address (e.g. ":8529")
	RelaySecret    string         `json:"relay_secret,omitempty"` // v5.4.1: shared secret for relay auth
	LogPaths       []string       `json:"log_paths,omitempty"`  // log files/dirs to tail and forward to the server
	LogEncrypt     bool           `json:"log_encrypt"`          // gzip+AES-256-GCM encrypt log uploads (default true)
	TLSSkipVerify  bool           `json:"tls_skip_verify,omitempty"` // skip server TLS cert verification (insecure; self-signed/lab only)
	CACert         string         `json:"ca_cert,omitempty"`          // path to a CA PEM bundle to trust (proper self-signed support)
	// ---- 新增采集器配置（可选，未配置时不启动）----
	RedfishTargets []RedfishTarget `json:"redfish_targets,omitempty"` // Redfish 硬件状态采集（服务器 BMC/iDRAC/iBMC）
	// OceanStor 不支持 Redfish，必须走 DeviceManager REST，因此是独立配置项
	OceanStorTargets []OceanStorTarget `json:"oceanstor_targets,omitempty"` // 华为 OceanStor 存储/磁盘框采集
	NetFlow          *NetFlowConfig    `json:"netflow,omitempty"`           // NetFlow 网络流量接收
	PacketCapture    *PacketConfig     `json:"packet_capture,omitempty"`    // 五元组包报文采集
	SNMP             *SNMPConfig       `json:"snmp,omitempty"`              // SNMP 轮询 + Trap 接收（网络设备纳管）
	// Hyper-V 虚拟机采集：默认在 Windows Hyper-V 宿主机上自动探测启用，无需配置
	HyperVIntervalSec int  `json:"hyperv_interval_sec,omitempty"` // 采集间隔(秒)，默认 60
	HyperVDisabled    bool `json:"hyperv_disabled,omitempty"`     // 显式关闭 Hyper-V 采集
}

func defaultConfig() config {
	py := "python3"
	if runtime.GOOS == "windows" {
		py = "python"
	}
	return config{
		Server: "http://localhost:8529",
		// 默认 30s/60s：10s/15s 对生产车队过于激进（每小时 360 次全量上报 + 每分钟数十次
		// 插件冷启动）。30s 是主流监控采样粒度（Prometheus/Zabbix 同量级），带宽降 3×、
		// 插件 spawn 降 4×。需要更实时的用户可在配置里下调 report_interval/plugin_interval。
		ReportInterval: 30,
		PluginInterval: 60,
		DiskPath:       defaultDiskPath(),
		PluginsDir:     "plugins",
		Python:         py,
		StateFile:      "agent_state.json",
		Category:       "",
		Token:          "",
		Listen:         ":8529",
		LogEncrypt:     true, // 日志加密上报默认开启
	}
}

func defaultDiskPath() string {
	if runtime.GOOS == "windows" {
		if d := os.Getenv("SystemDrive"); d != "" {
			return d + "\\"
		}
		return "C:\\"
	}
	return "/"
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := defaultConfig()

	// resolve config file path (manual scan so we can load before flag defaults)。
	// 默认自动探测 config.yaml / config.yml / config.json（第一个存在者）；YAML 为推荐格式，
	// 优先级最高，故新旧安装并存时（迁移期）以 YAML 为准。--config 显式指定则优先，
	// 且按其扩展名决定 JSON/YAML 解析。
	cfgPath := shared.ResolveConfigPath("config.yaml", "config.yml", "config.json")
	for i, a := range os.Args {
		if a == "--config" && i+1 < len(os.Args) {
			cfgPath = os.Args[i+1]
		}
	}
	// Load configuration: file-not-found is expected on first manual run, but
	// parse errors MUST surface — a silently-failed parse would leave the
	// agent pointing at the hardcoded default (localhost:8529), which is the
	// #1 cause of "agent reports to localhost" on freshly-installed Linux hosts
	// where the install script exited before writing config.
	if b, err := os.ReadFile(cfgPath); err == nil {
		if err := shared.DecodeConfig(cfgPath, b, &cfg); err != nil {
			slog.Error("配置文件解析失败，将使用默认值（localhost:8529）",
				"path", cfgPath, "err", err,
				"hint", "请检查配置文件 JSON/YAML 格式是否正确，或重新运行安装命令")
		} else {
			slog.Info("已加载配置文件", "path", cfgPath)
		}
	} else {
		slog.Warn("配置文件不存在，使用默认配置（localhost:8529）",
			"path", cfgPath,
			"hint", "请运行安装命令生成 config.yaml，或使用 --config 指定路径")
	}

	// 首次启动时在配置目录自动生成 config.example.yaml（已存在则跳过）
	ensureConfigExample(cfgPath)

	// flags override file/defaults
	var cfgFlag string
	flag.StringVar(&cfg.Server, "server", cfg.Server, "服务端地址，如 http://192.168.1.10:8529")
	flag.IntVar(&cfg.ReportInterval, "interval", cfg.ReportInterval, "基础指标上报间隔(秒)")
	flag.IntVar(&cfg.PluginInterval, "plugin-interval", cfg.PluginInterval, "插件执行周期(秒)")
	flag.StringVar(&cfg.DiskPath, "disk-path", cfg.DiskPath, "监控的磁盘路径")
	flag.StringVar(&cfg.PluginsDir, "plugins-dir", cfg.PluginsDir, "Python 插件目录")
	flag.StringVar(&cfg.Python, "python", cfg.Python, "运行 .py 插件的解释器")
	flag.StringVar(&cfg.Category, "category", cfg.Category, "主机分类标签，如 生产/测试/DB/办公终端")
	flag.StringVar(&cfg.Token, "token", cfg.Token, "安装 Token（由服务端安装命令注入，可选）")
	flag.BoolVar(&cfg.Relay, "relay", cfg.Relay, "网关中继模式：监听本地端口，转发所有请求到 --server 指定的云监控中心")
	flag.StringVar(&cfg.Listen, "listen", cfg.Listen, "Relay 监听地址，如 :8529")
	flag.StringVar(&cfg.RelaySecret, "relay-secret", cfg.RelaySecret, "Relay 共享密钥，用于上游服务端验证中继请求")
	flag.StringVar(&cfgFlag, "config", cfgPath, "配置文件路径")
	var logPathsFlag string
	flag.StringVar(&logPathsFlag, "log-paths", "", "采集的日志文件或目录路径，逗号分隔（如 /var/log/syslog,/var/log/nginx/）")
	flag.BoolVar(&cfg.LogEncrypt, "log-encrypt", cfg.LogEncrypt, "加密上报日志(gzip+AES-256-GCM)，默认开启；调试可设 --log-encrypt=false")
	flag.BoolVar(&cfg.TLSSkipVerify, "tls-skip-verify", cfg.TLSSkipVerify, "跳过服务端 TLS 证书校验（不安全，仅自签/内网临时使用）")
	flag.StringVar(&cfg.CACert, "ca-cert", cfg.CACert, "信任的 CA 证书路径（PEM），用于校验自签名服务端证书")
	var securityMode string
	flag.StringVar(&securityMode, "security-mode", "auto", "安全模块模式: auto(自动诊断输出修复命令)/permissive(自动切换宽容模式,2h后恢复)/enforcing(恢复强制模式)")
	flag.Parse()
	_ = cfgFlag
	if logPathsFlag != "" {
		for _, p := range strings.Split(logPathsFlag, ",") {
			if p = strings.TrimSpace(p); p != "" {
				cfg.LogPaths = append(cfg.LogPaths, p)
			}
		}
	}

	// Environment variable overrides (lowest priority: flag > env > config file > default).
	// Enables container / Kubernetes deployments where secrets are injected via env.
	if v := os.Getenv("AIOPS_SERVER"); v != "" {
		cfg.Server = v
	}
	if v := os.Getenv("AIOPS_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("AIOPS_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.ReportInterval = n
		}
	}
	if v := os.Getenv("AIOPS_CATEGORY"); v != "" {
		cfg.Category = v
	}
	if v := os.Getenv("AIOPS_PLUGINS_DIR"); v != "" {
		cfg.PluginsDir = v
	}
	if v := os.Getenv("AIOPS_STATE_FILE"); v != "" {
		cfg.StateFile = v
	}
	if v := os.Getenv("AIOPS_LOG_ENCRYPT"); v != "" {
		cfg.LogEncrypt = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("AIOPS_TLS_SKIP_VERIFY"); v != "" {
		cfg.TLSSkipVerify = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("AIOPS_CA_CERT"); v != "" {
		cfg.CACert = v
	}
	if v := os.Getenv("AIOPS_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("AIOPS_RELAY_SECRET"); v != "" {
		cfg.RelaySecret = v
	}

	// Apply server TLS trust (self-signed CA / skip-verify) to every agent→server
	// HTTP client before the first request is made.
	configureServerTLS(cfg.TLSSkipVerify, cfg.CACert)

	// Relay mode: act as a gateway for internal machines that can't reach the
	// internet. The agent listens on a local port and reverse-proxies to the
	// cloud server — only this one machine needs internet access.
	if cfg.Relay {
		listen := cfg.Listen
		if listen == "" {
			listen = ":8529"
		}
		runRelay(listen, strings.TrimRight(cfg.Server, "/"), cfg.RelaySecret)
		return
	}

	hostID := loadOrCreateHostID(cfg.StateFile)
	collector := newCollector(cfg.DiskPath)
	runner := NewPluginRunner(cfg.PluginsDir, cfg.Python, 15*time.Second)

	// v5.4.0: 安全环境检测（麒麟 kysec / SELinux / AppArmor / firewalld / Defender / SIP）
	// 启动时主动探测并输出诊断信息，让运维人员第一时间看到安全模块拦截风险。
	// 输出检测到的 OS 发行版信息
	osDist := getOSDist()
	if osDist.PrettyName != "" {
		slog.Info("检测到操作系统", "distro", osDist.PrettyName, "id", osDist.ID, "version", osDist.Version)
	} else if osDist.Name != "" {
		slog.Info("检测到操作系统", "name", osDist.Name, "id", osDist.ID, "version", osDist.Version)
	}

	if secModules, isKylin := detectSecurityEnv(); isKylin || len(secModules) > 0 {
		if isKylin {
			slog.Warn("检测到麒麟操作系统，请确认 kysec 安全模块不会拦截 Agent 数据采集",
				"os", runtime.GOOS, "distro", osDist.PrettyName)
		}
		var enforcingModules []SecurityModule
		for _, m := range secModules {
			level := slog.LevelInfo
			if m.Status == "enforcing" {
				level = slog.LevelWarn
				enforcingModules = append(enforcingModules, m)
			}
			slog.Log(nil, level, "检测到安全模块",
				"module", m.Name, "status", m.Status, "details", m.Details)
		}
		// Handle --security-mode parameter
		switch securityMode {
		case "permissive":
			if len(enforcingModules) > 0 {
				slog.Warn("安全模式=permissive，正在切换安全模块为宽容模式（2小时后自动恢复）")
				if err := setKysecMode("permissive", 2*time.Hour); err != nil {
					slog.Error("切换安全模块失败", "err", err)
				} else {
					slog.Info("安全模块已切换为宽容模式，2小时后自动恢复 enforcing")
				}
			}
		case "enforcing":
			slog.Info("安全模式=enforcing，正在恢复安全模块强制模式")
			if err := setKysecMode("enforcing", 0); err != nil {
				slog.Error("恢复安全模块失败", "err", err)
			} else {
				slog.Info("安全模块已恢复为 enforcing 模式")
			}
		case "auto":
			// Auto mode: output fix commands for any enforcing modules
			if len(enforcingModules) > 0 {
				cmds := securityFixCommands(enforcingModules)
				if len(cmds) > 0 {
					slog.Warn("检测到 enforcing 安全模块，以下是推荐的修复命令：")
					for _, cmd := range cmds {
						slog.Warn("  " + cmd)
					}
				}
			}
		}
		// Proactively check if procfs access is blocked
		if blocked := checkProcAccess(); len(blocked) > 0 {
			var paths []string
			for p := range blocked {
				paths = append(paths, p)
			}
			slog.Error("启动检测：部分 /proc 路径无法读取，数据采集可能不完整",
				"blocked_paths", paths,
				"hint", "请以 root 身份运行 Agent，或配置安全模块白名单",
			)
		}
	}

	// Resolve the effective server list: if "servers" is configured it takes
	// precedence; otherwise fall back to the legacy single "server" + "token".
	servers := cfg.Servers
	if len(servers) == 0 && cfg.Server != "" {
		servers = []ServerConfig{{Server: cfg.Server, Token: cfg.Token}}
	}
	if len(servers) == 0 {
		log.Fatal("未配置任何服务端地址（--server 或 servers 字段）")
	}
	// Guard: detect localhost target — a freshly-installed remote agent
	// connecting to its OWN localhost is the most common misconfiguration.
	// This typically means the config file was never written (install script failed
	// partway through, or the agent binary was copied without running the
	// install command). Relay mode is exempt: it listens locally by design.
	if !cfg.Relay {
		for _, sc := range servers {
			if strings.Contains(sc.Server, "localhost") || strings.Contains(sc.Server, "127.0.0.1") {
				slog.Error("Agent 上报地址为本地回环地址，远程连接必然失败！",
					"server", sc.Server,
					"config_path", cfgPath,
					"hint", "config.yaml 可能未正确生成。请在面板重新生成安装命令并执行，或手动编辑 config.yaml 的 server 字段为服务端实际可达地址")
			}
		}
	}
	// Log effective server(s) at startup for quick diagnosis
	for _, sc := range servers {
		slog.Info("Agent 上报目标", "server", sc.Server, "config_path", cfgPath)
	}
	agent := NewAgent(
		servers,
		time.Duration(cfg.ReportInterval)*time.Second,
		time.Duration(cfg.PluginInterval)*time.Second,
		collector, runner, hostID, cfg.Category,
	)
	agent.logPaths = cfg.LogPaths
	agent.logEncrypt = cfg.LogEncrypt
	agent.stateFile = cfg.StateFile // 认回规范 host_id 后要写回身份文件
	agent.redfishTargets = cfg.RedfishTargets
	agent.oceanStorTargets = cfg.OceanStorTargets
	agent.netflowCfg = cfg.NetFlow
	agent.packetCfg = cfg.PacketCapture
	agent.snmpCfg = cfg.SNMP
	agent.hypervInterval = time.Duration(cfg.HyperVIntervalSec) * time.Second
	agent.hypervDisabled = cfg.HyperVDisabled

	// Graceful shutdown: cancel context on SIGTERM/SIGINT, then wait for all
	// goroutines (report loop, plugin loop, terminal/forward channels, hardware
	// collectors) to drain before exiting. This prevents data loss on in-flight
	// reports and ensures the server sees a clean disconnect.
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		select {
		case <-sig:
			slog.Info("收到退出信号，正在优雅停止...")
			cancel()
		case <-ctx.Done():
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		agent.Run(ctx)
	}()

	wg.Wait()
	slog.Info("Agent 已完全停止。")
}
