package main

import (
	"fmt"
	"strings"
	"time"
)

// runAgentModule executes a built-in playbook module on one online host via the
// one-shot exec channel. Returns stdout body or an error.
func (s *Server) runAgentModule(hostID, module string, args map[string]string, timeoutSec int) (string, error) {
	hostID = strings.TrimSpace(hostID)
	module = strings.TrimSpace(module)
	if hostID == "" || module == "" {
		return "", fmt.Errorf("host_id and module required")
	}
	h := s.hostByID(hostID)
	if h == nil {
		return "", fmt.Errorf("host not found")
	}
	offlineSec := int64(s.cfg.Thresholds().OfflineAfter.Seconds())
	if time.Now().Unix()-h.LastSeen > offlineSec {
		return "", fmt.Errorf("host offline")
	}
	if timeoutSec < 5 {
		timeoutSec = 60
	}
	cmd := buildModuleCommand(module, args, nil)
	out, kind, err := s.execCommandOnHost(h, cmd, timeoutSec)
	if err != nil {
		if strings.TrimSpace(out) != "" {
			return out, fmt.Errorf("%s: %s", err.Error(), truncateRun(out, 400))
		}
		return out, err
	}
	if kind != execOK {
		return out, fmt.Errorf("module %s failed", module)
	}
	return out, nil
}

func truncateRun(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
