package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"log/slog"
)

// configExampleYAML is the embedded copy of config.example.yaml.
// It is written to the agent's config directory on first startup so users
// always have a complete, commented reference file next to their config.yaml.
// YAML is now the default/recommended config format (see ResolveConfigPath).
//
//go:generate go run copy_example.go
//go:embed config_example.yaml
var configExampleYAML []byte

// ensureConfigExample writes config.example.yaml next to the config file
// if it does not already exist. This guarantees every agent installation
// has a reference configuration regardless of install method.
func ensureConfigExample(cfgPath string) {
	dir := filepath.Dir(cfgPath)
	if dir == "" || dir == "." {
		// config in CWD — resolve absolute so we write to a real path
		if abs, err := filepath.Abs("."); err == nil {
			dir = abs
		}
	}
	dst := filepath.Join(dir, "config.example.yaml")
	if _, err := os.Stat(dst); err == nil {
		return // already exists — never overwrite user edits
	}
	if err := os.WriteFile(dst, configExampleYAML, 0644); err != nil {
		// read-only rootfs (compose demo agent) is expected — don't scare operators.
		msg := err.Error()
		if strings.Contains(msg, "read-only") || strings.Contains(msg, "permission denied") {
			return
		}
		slog.Warn("写入 config.example.yaml 失败", "path", dst, "err", err)
	} else {
		slog.Info("已生成配置示例文件", "path", dst)
	}
}
