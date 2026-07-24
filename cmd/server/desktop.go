package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// resolveDesktopTarget picks RDP/VNC protocol + port for a host.
func resolveDesktopTarget(h *Host) (protocol string, port int, listening bool) {
	osName := strings.ToLower(h.OS)
	if h.Desktop != nil {
		if h.Desktop.PreferredPort > 0 {
			proto := h.Desktop.Preferred
			if proto == "" {
				if h.Desktop.RDP {
					proto = "rdp"
				} else {
					proto = "vnc"
				}
			}
			listening = len(h.Desktop.Ports) > 0
			return proto, h.Desktop.PreferredPort, listening
		}
		if h.Desktop.RDP {
			return "rdp", 3389, true
		}
		if h.Desktop.VNC {
			p := 5900
			for _, x := range h.Desktop.Ports {
				if x >= 5900 && x <= 5999 {
					p = x
					break
				}
			}
			return "vnc", p, true
		}
	}
	switch {
	case strings.Contains(osName, "windows"):
		return "rdp", 3389, false
	case strings.Contains(osName, "darwin"), strings.Contains(osName, "mac"):
		return "vnc", 5900, false
	default:
		return "vnc", 5900, false
	}
}

func (s *Server) desktopConnectHost(r *http.Request) string {
	if pub := strings.TrimSpace(s.cfg.Get().PublicURL); pub != "" {
		if u, err := url.Parse(pub); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func buildRDPFile(fullAddress, hostname string) string {
	// Minimal .rdp for mstsc / Windows App / Microsoft Remote Desktop
	var b strings.Builder
	b.WriteString("full address:s:" + fullAddress + "\r\n")
	b.WriteString("prompt for credentials:i:1\r\n")
	b.WriteString("administrative session:i:0\r\n")
	b.WriteString("screen mode id:i:2\r\n")
	b.WriteString("desktopwidth:i:1920\r\n")
	b.WriteString("desktopheight:i:1080\r\n")
	b.WriteString("session bpp:i:32\r\n")
	b.WriteString("compression:i:1\r\n")
	b.WriteString("authentication level:i:2\r\n")
	b.WriteString("negotiate security layer:i:1\r\n")
	b.WriteString("enablecredsspsupport:i:1\r\n")
	if hostname != "" {
		b.WriteString("username:s:\r\n")
	}
	return b.String()
}

func (s *Server) findDesktopForward(hostID string, targetPort int) *forwardInfo {
	for _, info := range s.forward.listRules() {
		if info.HostID == hostID && info.TargetPort == targetPort && info.Enabled && (info.Protocol == "" || info.Protocol == "tcp") {
			cp := info
			return &cp
		}
	}
	return nil
}

// handleOpenDesktop creates (or reuses) a TCP reverse tunnel to the host's
// RDP/VNC port and returns connection details for the operator's local client.
// POST /api/v1/hosts/{id}/desktop
// Requires the same terminal secondary verification as the remote shell.
func (s *Server) handleOpenDesktop(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.ForwardEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "forward.disabled")})
		return
	}
	if verified, _ := s.auth.isTerminalVerified(r); !verified {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": Tr(r, "terminal_auth.terminal_verify_required"),
			"code":  "terminal_verify_required",
		})
		return
	}
	id := r.PathValue("id")
	h, ok := s.store.GetHost(id)
	if !ok || h == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.host_not_found")})
		return
	}
	proto, port, listening := resolveDesktopTarget(h)
	if port < 1 || port > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid desktop port"})
		return
	}

	info := s.findDesktopForward(id, port)
	if info == nil {
		listenHost := s.cfg.ForwardListenAddr()
		operator, _ := s.actorIP(r)
		rule, err := s.forward.createRule(id, h.Hostname, port, 0, listenHost, "tcp", "", operator, "")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		go s.serveRule(rule)
		info = &forwardInfo{
			ID: rule.id, HostID: rule.hostID, Hostname: rule.hostname,
			TargetPort: rule.targetPort, LocalPort: rule.localPort,
			ListenAddr: rule.listenAddr, Status: "active",
			CreatedAt: rule.createdAt, Operator: operator, Protocol: rule.protocol,
		}
	}

	connectHost := s.desktopConnectHost(r)
	// If the forward binds only to loopback, clients must use 127.0.0.1
	if strings.HasPrefix(info.ListenAddr, "127.0.0.1:") || strings.HasPrefix(info.ListenAddr, "localhost:") || strings.HasPrefix(info.ListenAddr, "[::1]:") {
		connectHost = "127.0.0.1"
	}
	fullAddr := net.JoinHostPort(connectHost, strconv.Itoa(info.LocalPort))

	resp := map[string]any{
		"status":         "ok",
		"host_id":        id,
		"hostname":       h.Hostname,
		"os":             h.OS,
		"protocol":       proto,
		"target_port":    port,
		"local_port":     info.LocalPort,
		"listen_addr":    info.ListenAddr,
		"connect_host":   connectHost,
		"connect_addr":   fullAddr,
		"forward_id":     info.ID,
		"service_listening": listening,
		"desktop":        h.Desktop,
	}
	switch proto {
	case "rdp":
		resp["rdp_file"] = buildRDPFile(fullAddr, h.Hostname)
		resp["rdp_filename"] = sanitizeDesktopFilename(h.Hostname) + ".rdp"
		resp["hint"] = "rdp"
	default:
		resp["vnc_url"] = "vnc://" + fullAddr
		resp["hint"] = "vnc"
	}

	actor, clientIP := s.actorIP(r)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: actor, IP: clientIP, Host: h.Hostname,
		Message: fmt.Sprintf("open desktop %s → %s (%s :%d)", shortID(id), fullAddr, proto, port)})
	writeJSON(w, http.StatusOK, resp)
}

func sanitizeDesktopFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "remote-desktop"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		default:
			// skip
		}
	}
	out := b.String()
	if out == "" {
		return "remote-desktop"
	}
	return out
}
