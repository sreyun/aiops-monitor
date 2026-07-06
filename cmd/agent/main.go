package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

type config struct {
	Server         string `json:"server"`
	ReportInterval int    `json:"report_interval"`
	PluginInterval int    `json:"plugin_interval"`
	DiskPath       string `json:"disk_path"`
	PluginsDir     string `json:"plugins_dir"`
	Python         string `json:"python"`
	StateFile      string `json:"state_file"`
	Category       string `json:"category"`
	Token          string `json:"token"`
}

func defaultConfig() config {
	py := "python3"
	if runtime.GOOS == "windows" {
		py = "python"
	}
	return config{
		Server:         "http://localhost:8080",
		ReportInterval: 5,
		PluginInterval: 15,
		DiskPath:       defaultDiskPath(),
		PluginsDir:     "plugins",
		Python:         py,
		StateFile:      "agent_state.json",
		Category:       "",
		Token:          "",
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
		log.Printf("已加载配置文件: %s", cfgPath)
	}

	// flags override file/defaults
	var cfgFlag string
	flag.StringVar(&cfg.Server, "server", cfg.Server, "服务端地址，如 http://192.168.1.10:8080")
	flag.IntVar(&cfg.ReportInterval, "interval", cfg.ReportInterval, "基础指标上报间隔(秒)")
	flag.IntVar(&cfg.PluginInterval, "plugin-interval", cfg.PluginInterval, "插件执行周期(秒)")
	flag.StringVar(&cfg.DiskPath, "disk-path", cfg.DiskPath, "监控的磁盘路径")
	flag.StringVar(&cfg.PluginsDir, "plugins-dir", cfg.PluginsDir, "Python 插件目录")
	flag.StringVar(&cfg.Python, "python", cfg.Python, "运行 .py 插件的解释器")
	flag.StringVar(&cfg.Category, "category", cfg.Category, "主机分类标签，如 生产/测试/DB/办公终端")
	flag.StringVar(&cfg.Token, "token", cfg.Token, "安装 Token（由服务端安装命令注入，可选）")
	flag.StringVar(&cfgFlag, "config", cfgPath, "配置文件路径")
	flag.Parse()
	_ = cfgFlag

	hostID := loadOrCreateHostID(cfg.StateFile)
	collector := newCollector(cfg.DiskPath)
	runner := NewPluginRunner(cfg.PluginsDir, cfg.Python, 15*time.Second)
	agent := NewAgent(
		cfg.Server,
		time.Duration(cfg.ReportInterval)*time.Second,
		time.Duration(cfg.PluginInterval)*time.Second,
		collector, runner, hostID, cfg.Category, cfg.Token,
	)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		log.Printf("收到退出信号，Agent 停止。")
		os.Exit(0)
	}()

	agent.Run()
}
