package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Port forwarding — agent side.
//
// The agent long-polls the server for forward session requests. When one
// arrives, the agent dials localhost:targetPort and opens two HTTP streams:
//   - rx (server→agent): framed user data, relayed to the TCP connection
//   - tx (agent→server): raw target service output, relayed to the user
//
// This mirrors the terminal reverse channel architecture but is completely
// independent — separate endpoints, separate polling loop, separate session
// handling. The two features share zero code paths.

// forwardWaitHTTP bounds the long-poll so a half-open network can't wedge the
// poller. Slightly above the server's 25s poll timeout, matching termWaitHTTP.
var forwardWaitHTTP = &http.Client{Timeout: 35 * time.Second}

// runForwardChannelFor runs a persistent reverse forward channel for one
// server target. Each target gets its own goroutine so forward sessions from
// different servers don't interfere.
func (a *Agent) runForwardChannelFor(t *serverTarget) {
	if a.identity.Fingerprint == "" {
		slog.Warn("端口转发通道未启用：未采集到机器指纹", "server", t.server)
		return
	}
	slog.Info("端口转发通道已就绪，等待服务端呼叫…", "server", t.server)
	for {
		sid, targetPort, ok := a.forwardWait(t.server)
		if !ok {
			time.Sleep(3 * time.Second)
			continue
		}
		if sid == "" {
			continue // long-poll timeout, re-poll immediately
		}
		go a.runForwardSession(t.server, sid, targetPort)
	}
}

// forwardWait long-polls the server for a pending forward session.
func (a *Agent) forwardWait(server string) (sessionID string, targetPort int, ok bool) {
	q := url.Values{"host": {a.identity.HostID}, "fp": {a.identity.Fingerprint}}
	resp, err := forwardWaitHTTP.Get(server + "/api/v1/agent/forward/wait?" + q.Encode())
	if err != nil {
		return "", 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, false
	}
	var out struct {
		Session    string `json:"session"`
		TargetPort int    `json:"target_port"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Session, out.TargetPort, true
}

// runForwardSession dials localhost:targetPort and relays data between the TCP
// connection and the server's rx/tx streams. A panic in this goroutine must
// never crash the whole agent.
func (a *Agent) runForwardSession(server, sid string, targetPort int) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("转发会话异常已恢复（不影响 Agent 运行）", "session", sid, "panic", r)
		}
	}()
	target := "localhost:" + strconv.Itoa(targetPort)
	conn, err := net.Dial("tcp", target)
	if err != nil {
		slog.Warn("转发目标连接失败", "session", sid, "target", target, "err", err)
		// Notify the server by closing the tx stream immediately (empty body).
		fp := url.QueryEscape(a.identity.Fingerprint)
		req, _ := http.NewRequest("POST",
			server+"/api/v1/agent/forward/tx?session="+sid+"&fp="+fp, nil)
		req.Header.Set("Content-Type", "application/octet-stream")
		if resp, err := termHTTP.Do(req); err == nil {
			resp.Body.Close()
		}
		return
	}
	slog.Info("转发会话开始", "session", sid, "target", target)
	fp := url.QueryEscape(a.identity.Fingerprint)

	var once sync.Once
	closeAll := func() { once.Do(func() { _ = conn.Close() }) }
	defer closeAll()

	var wg sync.WaitGroup
	wg.Add(2)

	// tx: stream target service output up (POST body ends when conn closes)
	go func() {
		defer wg.Done()
		defer closeAll()
		req, err := http.NewRequest("POST",
			server+"/api/v1/agent/forward/tx?session="+sid+"&fp="+fp, conn)
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		if resp, err := termHTTP.Do(req); err == nil {
			resp.Body.Close()
		}
	}()

	// rx: framed user data from the server → the TCP connection
	go func() {
		defer wg.Done()
		defer closeAll()
		resp, err := termHTTP.Get(server + "/api/v1/agent/forward/rx?session=" + sid + "&fp=" + fp)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		readForwardFrames(resp.Body, conn)
	}()

	wg.Wait()
	slog.Info("转发会话结束", "session", sid, "target", target)
}

// readForwardFrames parses the rx stream: each frame is [type:1][len:2 BE][payload].
// type 'd' = data bytes, 'c' = close signal.
func readForwardFrames(r io.Reader, conn net.Conn) {
	var hdr [3]byte
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return
		}
		n := int(binary.BigEndian.Uint16(hdr[1:]))
		payload := make([]byte, n)
		if n > 0 {
			if _, err := io.ReadFull(r, payload); err != nil {
				return
			}
		}
		switch hdr[0] {
		case 'd':
			if _, err := conn.Write(payload); err != nil {
				return
			}
		case 'c':
			return // close signal from the server
		}
	}
}
