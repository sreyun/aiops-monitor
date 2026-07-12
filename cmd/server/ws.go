package main

import (
	"bufio"
	"crypto/sha1" // #nosec G505 -- WebSocket 握手(RFC 6455)强制用 SHA-1 生成 Sec-WebSocket-Accept，非加密用途
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// Minimal RFC 6455 WebSocket server implementation (stdlib-only). It carries the
// browser⇄server hop of the remote terminal: the browser opens a WebSocket, the
// server relays bytes to/from the agent over plain HTTP streams. Only the frame
// features the terminal needs are implemented (text/binary/ping/pong/close, no
// permessage-deflate, minimal fragmentation handling).

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	wsOpText   = 0x1
	wsOpBinary = 0x2
	wsOpClose  = 0x8
	wsOpPing   = 0x9
	wsOpPong   = 0xA
)

type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
	wmu  sync.Mutex // serialize writes (read loop may emit pong/close)
}

// wsAccept upgrades an HTTP request to a WebSocket connection by hijacking it.
func wsAccept(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") ||
		!strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("not a websocket upgrade")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("response writer does not support hijack")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := brw.WriteString(resp); err != nil {
		conn.Close()
		return nil, err
	}
	if err := brw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	return &wsConn{conn: conn, br: brw.Reader}, nil
}

// ReadMessage returns the next application (text/binary) message payload,
// transparently answering pings and surfacing a close as io.EOF. Continuation
// frames are reassembled.
func (c *wsConn) ReadMessage() ([]byte, error) {
	var msg []byte
	for {
		fin, opcode, payload, err := c.readFrame()
		if err != nil {
			return nil, err
		}
		switch opcode {
		case wsOpText, wsOpBinary, 0x0: // 0x0 = continuation
			msg = append(msg, payload...)
			if fin {
				return msg, nil
			}
		case wsOpPing:
			_ = c.writeFrame(wsOpPong, payload)
		case wsOpPong:
			// ignore
		case wsOpClose:
			_ = c.writeFrame(wsOpClose, nil)
			return nil, io.EOF
		}
	}
}

func (c *wsConn) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(c.br, hdr[:]); err != nil {
		return
	}
	fin = hdr[0]&0x80 != 0
	opcode = hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	ln := int(hdr[1] & 0x7f)
	switch ln {
	case 126:
		var e [2]byte
		if _, err = io.ReadFull(c.br, e[:]); err != nil {
			return
		}
		ln = int(binary.BigEndian.Uint16(e[:]))
	case 127:
		var e [8]byte
		if _, err = io.ReadFull(c.br, e[:]); err != nil {
			return
		}
		ln = int(binary.BigEndian.Uint64(e[:]))
	}
	if ln < 0 || ln > 8<<20 { // 8 MiB frame cap
		err = errors.New("websocket frame too large")
		return
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(c.br, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, ln)
	if _, err = io.ReadFull(c.br, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return
}

// WriteText sends a text frame (server→client frames are never masked).
func (c *wsConn) WriteText(b []byte) error { return c.writeFrame(wsOpText, b) }

// WriteBinary sends a binary frame — used for raw shell output that may not be
// valid UTF-8.
func (c *wsConn) WriteBinary(b []byte) error { return c.writeFrame(wsOpBinary, b) }

func (c *wsConn) writeFrame(opcode byte, data []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	var hdr []byte
	b0 := byte(0x80 | opcode) // FIN + opcode
	n := len(data)
	switch {
	case n < 126:
		hdr = []byte{b0, byte(n)}
	case n < 65536:
		hdr = []byte{b0, 126, byte(n >> 8), byte(n)}
	default:
		hdr = make([]byte, 10)
		hdr[0], hdr[1] = b0, 127
		binary.BigEndian.PutUint64(hdr[2:], uint64(n))
	}
	if _, err := c.conn.Write(hdr); err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	_, err := c.conn.Write(data)
	return err
}

func (c *wsConn) Close() error { return c.conn.Close() }
