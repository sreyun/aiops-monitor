package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// appVersion is shown in the dashboard sidebar and the summary API.
const appVersion = "1.1.0"

// resolveDist finds the directory that holds the downloadable agent binaries
// (+ plugins.zip). It tries the -dist flag, ./dist, then the server executable's
// own dir and its dist/ subdir — so the one-line install works whether the
// server is launched from the repo root or from bin/.
func resolveDist(flagVal string) string {
	var candidates []string
	if flagVal != "" {
		candidates = append(candidates, flagVal)
	}
	candidates = append(candidates, "dist")
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(d, "dist"), d)
	}
	for _, c := range candidates {
		if hasAgentBinary(c) {
			return c
		}
	}
	if flagVal != "" {
		return flagVal
	}
	return "dist"
}

func hasAgentBinary(dir string) bool {
	if dir == "" {
		return false
	}
	for _, n := range []string{
		"aiops-agent.exe",
		"aiops-agent-linux-amd64", "aiops-agent-linux-arm64",
		"aiops-agent-darwin-arm64", "aiops-agent-darwin-amd64",
	} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

// corsMiddleware allows the dashboard (or external tools) to call the API
// cross-origin and short-circuits preflight OPTIONS requests.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	addr := flag.String("addr", ":8080", "监听地址，如 :8080 或 0.0.0.0:8080")
	cfgPath := flag.String("config", "server_config.json", "服务端配置文件路径（告警/阈值/分类）")
	distDir := flag.String("dist", "", "Agent 下载目录（含各平台二进制与 plugins.zip）；留空自动探测 ./dist 或程序所在目录")
	flag.Parse()

	dist := resolveDist(*distDir)
	store := NewStore()
	cfg := NewConfigStore(*cfgPath)
	notifier := NewNotifier(store, cfg)
	server := NewServer(store, cfg, notifier, dist, *addr)

	// embedded lightweight DB: restore state, then autosave + flush on exit
	db := NewDB(dbPathFor(*cfgPath), store, server.auth)
	db.Load()
	go db.AutoSave(15 * time.Second)
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		if err := db.Save(); err != nil {
			log.Printf("退出前落盘失败: %v", err)
		} else {
			log.Printf("状态已落盘,服务端退出。")
		}
		os.Exit(0)
	}()

	go notifier.Run(10 * time.Second)     // periodic alert evaluation + dedup push
	go server.checks.Run(5 * time.Second) // custom HTTP/TCP synthetic checks

	handler := corsMiddleware(server.authMiddleware(server.Routes()))
	srv := &http.Server{
		Addr:         *addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	log.Printf("AIOps Monitor 服务端已启动")
	log.Printf("  监控面板: http://localhost%s", *addr)
	log.Printf("  API 前缀: http://localhost%s/api/v1/", *addr)
	log.Printf("  配置文件: %s", *cfgPath)
	log.Printf("  数据库:   %s（内嵌轻量库,历史/日志/会话重启不丢）", dbPathFor(*cfgPath))
	if hasAgentBinary(dist) {
		log.Printf("  下载目录: %s（一键安装可用）", dist)
	} else {
		log.Printf("  警告: 未找到 Agent 下载文件，一键安装不可用；请用 -dist 指定含 aiops-agent* 与 plugins.zip 的目录")
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
