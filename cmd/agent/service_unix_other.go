//go:build !windows && !linux && !darwin

package main

import "fmt"

// Other Unix platforms have no packaged daemon integration yet; run the agent
// directly (e.g. under your own init/rc script).

const agentServiceName = "aiops-monitor-agent"

func installAgentService(exePath, cfgPath string) error {
	return fmt.Errorf("--install-service 暂不支持当前平台")
}

func uninstallAgentService() error {
	return fmt.Errorf("--uninstall-service 暂不支持当前平台")
}

func runAgentAsService(agent *Agent, cfgPath string) error {
	return fmt.Errorf("--service 暂不支持当前平台")
}

func runDesktopWorker(agent *Agent) error {
	return fmt.Errorf("--desktop-worker 暂不支持当前平台")
}
