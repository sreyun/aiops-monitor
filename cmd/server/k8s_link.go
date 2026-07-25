package main

import (
	"strings"
)

// matchHostForK8sNode finds a managed host by node name or addresses (InternalIP/Hostname).
func (s *Server) matchHostForK8sNode(nodeName string, addrs []string) *Host {
	byName := map[string]*Host{}
	byIP := map[string]*Host{}
	for _, h := range s.store.ListHosts() {
		if h.Hostname != "" {
			byName[strings.ToLower(h.Hostname)] = h
		}
		if h.IP != "" {
			byIP[h.IP] = h
		}
	}
	if nodeName != "" {
		ln := strings.ToLower(nodeName)
		if h := byName[ln]; h != nil {
			return h
		}
		// short hostname vs FQDN (either side)
		if i := strings.IndexByte(ln, '.'); i > 0 {
			if h := byName[ln[:i]]; h != nil {
				return h
			}
		}
		for name, h := range byName {
			if i := strings.IndexByte(name, '.'); i > 0 && name[:i] == ln {
				return h
			}
			if i := strings.IndexByte(ln, '.'); i > 0 && ln[:i] == name {
				return h
			}
		}
	}
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if h := byIP[a]; h != nil {
			return h
		}
		if h := byName[strings.ToLower(a)]; h != nil {
			return h
		}
	}
	return nil
}

func k8sNodeAddresses(obj map[string]any) (ips []string, hostnames []string) {
	st, _ := obj["status"].(map[string]any)
	if st == nil {
		return nil, nil
	}
	raw, _ := st["addresses"].([]any)
	for _, it := range raw {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		typ, _ := m["type"].(string)
		addr, _ := m["address"].(string)
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		switch typ {
		case "InternalIP", "ExternalIP":
			ips = append(ips, addr)
		case "Hostname":
			hostnames = append(hostnames, addr)
		}
	}
	return ips, hostnames
}

func (s *Server) enrichK8sNodeRow(it map[string]any, row map[string]any) {
	_, name := k8sMetaName(it)
	ips, hostnames := k8sNodeAddresses(it)
	addrs := append(append([]string{}, ips...), hostnames...)
	if h := s.matchHostForK8sNode(name, addrs); h != nil {
		row["linked_host_id"] = h.ID
		row["linked_host_name"] = h.Hostname
	}
	if len(ips) > 0 {
		row["internal_ip"] = ips[0]
	}
}

func (s *Server) hostIDForK8sNodeName(nodeName string) (string, string) {
	if h := s.matchHostForK8sNode(nodeName, nil); h != nil {
		return h.ID, h.Hostname
	}
	return "", ""
}
