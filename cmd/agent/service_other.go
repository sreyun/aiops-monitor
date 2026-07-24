//go:build !windows

package main

import "fmt"

// The privileged Windows-service + secure-desktop worker model only applies to
// Windows. On other platforms these are no-ops so main.go stays cross-platform.

const agentServiceName = "AiopsMonitorAgent"

func installAgentService(exePath, cfgPath string) error {
	return fmt.Errorf("--install-service 仅支持 Windows")
}

func uninstallAgentService() error {
	return fmt.Errorf("--uninstall-service 仅支持 Windows")
}

func runAgentAsService(agent *Agent, cfgPath string) error {
	return fmt.Errorf("--service 仅支持 Windows")
}

func runDesktopWorker(agent *Agent) error {
	return fmt.Errorf("--desktop-worker 仅支持 Windows")
}
