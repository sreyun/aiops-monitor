package main

import (
	"fmt"
	"strings"
	"time"
)

// runAgentModule executes a built-in playbook module on one online host via the
// one-shot exec channel. Returns stdout body or an error.
func (s *Server) runAgentModule(hostID, module string, args map[string]string, timeoutSec int) (string, error) {
	out, kind, err := s.runAgentModuleKind(hostID, module, args, timeoutSec)
	if err != nil {
		return out, err
	}
	if kind != execOK {
		return out, fmt.Errorf("module %s failed", module)
	}
	return out, nil
}

// runAgentModuleKind is like runAgentModule but preserves execKind for retry decisions.
func (s *Server) runAgentModuleKind(hostID, module string, args map[string]string, timeoutSec int) (string, execKind, error) {
	hostID = strings.TrimSpace(hostID)
	module = strings.TrimSpace(module)
	if hostID == "" || module == "" {
		return "", execExit, fmt.Errorf("host_id and module required")
	}
	h := s.hostByID(hostID)
	if h == nil {
		return "", execExit, fmt.Errorf("host not found")
	}
	offlineSec := int64(s.cfg.Thresholds().OfflineAfter.Seconds())
	if time.Now().Unix()-h.LastSeen > offlineSec {
		return "", execExit, fmt.Errorf("host offline")
	}
	if timeoutSec < 5 {
		timeoutSec = 60
	}
	cmd := buildModuleCommand(module, args, nil)
	out, kind, err := s.execCommandOnHost(h, cmd, timeoutSec)
	if err != nil {
		if strings.TrimSpace(out) != "" {
			return out, kind, fmt.Errorf("%s: %s", err.Error(), truncateRun(out, 400))
		}
		return out, kind, err
	}
	if kind != execOK {
		return out, kind, fmt.Errorf("module %s failed", module)
	}
	return out, kind, nil
}

func truncateRun(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
