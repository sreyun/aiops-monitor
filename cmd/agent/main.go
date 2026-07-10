package main

import (
	"encoding/json"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
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
	LogPaths       []string       `json:"log_paths,omitempty"`  // log files to tail and forward to the server
}

func defaultConfig() config {
	py := "python3"
	if runtime.GOOS == "windows" {
		py = "python"
	}
	return config{
		Server:         "http://localhost:8529",
		ReportInterval: 10,
		PluginInterval: 15,
		DiskPath:       defaultDiskPath(),
		PluginsDir:     "plugins",
		Python:         py,
		StateFile:      "agent_state.json",
		Category:       "",
		Token:          "",
		Listen:         ":8529",
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

	// resolve config file path (manual scan so we can load before flag defaults)
	cfgPath := "config.json"
	for i, a := range os.Args {
		if a == "--config" && i+1 < len(os.Args) {
			cfgPath = os.Args[i+1]
		}
	}
	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(b, &cfg)
		slog.Info("已加载配置文件", "path", cfgPath)
	}

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
	flag.StringVar(&logPathsFlag, "log-paths", "", "采集的日志文件路径，逗号分隔（如 /var/log/syslog,/var/log/nginx/error.log）")
	flag.Parse()
	_ = cfgFlag
	if logPathsFlag != "" {
		for _, p := range strings.Split(logPathsFlag, ",") {
			if p = strings.TrimSpace(p); p != "" {
				cfg.LogPaths = append(cfg.LogPaths, p)
			}
		}
	}

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

	// Resolve the effective server list: if "servers" is configured it takes
	// precedence; otherwise fall back to the legacy single "server" + "token".
	servers := cfg.Servers
	if len(servers) == 0 && cfg.Server != "" {
		servers = []ServerConfig{{Server: cfg.Server, Token: cfg.Token}}
	}
	if len(servers) == 0 {
		log.Fatal("未配置任何服务端地址（--server 或 servers 字段）")
	}
	agent := NewAgent(
		servers,
		time.Duration(cfg.ReportInterval)*time.Second,
		time.Duration(cfg.PluginInterval)*time.Second,
		collector, runner, hostID, cfg.Category,
	)
	agent.logPaths = cfg.LogPaths

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		slog.Info("收到退出信号，Agent 停止。")
		os.Exit(0)
	}()

	agent.Run()
}
