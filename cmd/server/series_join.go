package main

import (
	"sort"
)

// Presence-aware join helpers for multi-series Prom/VM exports reassembled by
// exact timestamp. Without presence+LOCF, missing series paint as Go zeros
// (fake 0ms / false offline) and charts flicker between N curves.

// checkJoinCell rebuilds CheckPoint with presence tracking.
type checkJoinCell struct {
	ts  int64
	p   CheckPoint
	set map[string]bool
}

func (c *checkJoinCell) mark(k string) {
	if c.set == nil {
		c.set = map[string]bool{}
	}
	c.set[k] = true
}

func finalizeCheckJoin(byTs map[int64]*checkJoinCell) []CheckPoint {
	if len(byTs) == 0 {
		return nil
	}
	cells := make([]*checkJoinCell, 0, len(byTs))
	for _, c := range byTs {
		cells = append(cells, c)
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].ts < cells[j].ts })

	var (
		lastOK                              *bool
		lastLat, lastLoss                   *float64
		lastCode                            *int
		lastDNS, lastTCP, lastTLS, lastTTFB *float64
		lastCert, lastBytes                 *float64
	)
	out := make([]CheckPoint, 0, len(cells))
	for _, c := range cells {
		locfF := func(key string, dst *float64, last **float64) {
			if c.set[key] {
				v := *dst
				*last = &v
				return
			}
			if *last != nil {
				*dst = **last
			}
		}
		locfI := func(key string, dst *int, last **int) {
			if c.set[key] {
				v := *dst
				*last = &v
				return
			}
			if *last != nil {
				*dst = **last
			}
		}

		if c.set["ok"] {
			v := c.p.OK
			lastOK = &v
		} else if lastOK != nil {
			c.p.OK = *lastOK
		}
		locfF("latency_ms", &c.p.LatencyMs, &lastLat)
		locfI("status_code", &c.p.StatusCode, &lastCode)
		// LossPct uses -1 sentinel when never seen; only LOCF once a real value arrived.
		if c.set["loss_pct"] {
			v := c.p.LossPct
			lastLoss = &v
		} else if lastLoss != nil {
			c.p.LossPct = *lastLoss
		}
		locfF("dns_ms", &c.p.DnsMs, &lastDNS)
		locfF("tcp_ms", &c.p.TcpMs, &lastTCP)
		locfF("tls_ms", &c.p.TlsMs, &lastTLS)
		locfF("ttfb_ms", &c.p.TtfbMs, &lastTTFB)
		locfF("cert_days", &c.p.CertDays, &lastCert)
		locfF("resp_bytes", &c.p.RespBytes, &lastBytes)

		c.p.Ts = c.ts
		out = append(out, c.p)
	}
	return out
}

// apiJoinCell rebuilds APIHistPoint with presence tracking.
type apiJoinCell struct {
	ts  int64
	p   APIHistPoint
	set map[string]bool
}

func (c *apiJoinCell) mark(k string) {
	if c.set == nil {
		c.set = map[string]bool{}
	}
	c.set[k] = true
}

func finalizeAPIJoin(byTs map[int64]*apiJoinCell) []APIHistPoint {
	if len(byTs) == 0 {
		return nil
	}
	cells := make([]*apiJoinCell, 0, len(byTs))
	for _, c := range byTs {
		cells = append(cells, c)
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].ts < cells[j].ts })

	var (
		lastOK                                                  *bool
		lastLat, lastDNS, lastTCP, lastTLS, lastTTFB, lastBytes *float64
		lastCode                                                *int
	)
	out := make([]APIHistPoint, 0, len(cells))
	for _, c := range cells {
		locfF := func(key string, dst *float64, last **float64) {
			if c.set[key] {
				v := *dst
				*last = &v
				return
			}
			if *last != nil {
				*dst = **last
			}
		}
		locfI := func(key string, dst *int, last **int) {
			if c.set[key] {
				v := *dst
				*last = &v
				return
			}
			if *last != nil {
				*dst = **last
			}
		}

		if c.set["ok"] {
			v := c.p.OK
			lastOK = &v
		} else if lastOK != nil {
			c.p.OK = *lastOK
		}
		locfF("latency_ms", &c.p.LatencyMs, &lastLat)
		locfI("status_code", &c.p.StatusCode, &lastCode)
		locfF("dns_ms", &c.p.DnsMs, &lastDNS)
		locfF("tcp_ms", &c.p.TcpMs, &lastTCP)
		locfF("tls_ms", &c.p.TlsMs, &lastTLS)
		locfF("ttfb_ms", &c.p.TtfbMs, &lastTTFB)
		locfF("resp_bytes", &c.p.RespBytes, &lastBytes)

		c.p.Ts = c.ts
		out = append(out, c.p)
	}
	return out
}
