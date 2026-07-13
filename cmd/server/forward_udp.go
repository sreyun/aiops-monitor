package main

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

// UDP 端口转发 —— 服务端。
//
// UDP 无连接：一个监听 socket 会收到多个客户端的数据报，这里按「客户端源地址」拆成独立
// flow，每个 flow 复用一条 forwardSession（经 agent 反向通道隧道到目标主机 localhost:port
// 的 UDP 服务）。与 TCP 的关键区别是两个方向都必须保留数据报边界：
//   - 下行 server→agent：本就按帧 'd' 封装（forwardFrame），agent 收到一帧写一个数据报；
//   - 上行 agent→server：由 agent 也按帧封装，readForwardTxFrames 逐帧还原为数据报回投客户端。

// forwardUDPFlowIdle 是单个 UDP flow 的空闲回收阈值。UDP 无关闭信号，长时间无双向流量即回收。
const forwardUDPFlowIdle = 90 * time.Second

// serveForwardUDP 运行一条 UDP 转发规则：读客户端数据报，按源地址多路复用到会话，
// 并把目标回程数据报投回对应客户端。packetConn 关闭（规则停用/删除）时循环退出。
func (s *Server) serveForwardUDP(rule *forwardRule) {
	pc := rule.packetConn
	if pc == nil {
		return
	}
	var mu sync.Mutex
	flows := map[string]*forwardSession{} // clientAddr.String() → session
	buf := make([]byte, 64*1024)
	for {
		n, clientAddr, err := pc.ReadFrom(buf)
		if err != nil {
			return // packetConn 已关闭
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		key := clientAddr.String()
		mu.Lock()
		sess := flows[key]
		mu.Unlock()
		if sess == nil {
			var cerr error
			sess, cerr = s.forward.createSession(rule.id, rule.hostID, rule.hostname, rule.targetPort, "udp", rule.operator)
			if cerr != nil {
				continue // 达到会话上限：丢弃该数据报
			}
			mu.Lock()
			flows[key] = sess
			mu.Unlock()
			if !s.forward.notifyAgent(rule.hostID, forwardWaitInfo{sessionID: sess.id, targetPort: rule.targetPort, mode: "udp"}) {
				s.forward.removeSession(sess.id)
				mu.Lock()
				delete(flows, key)
				mu.Unlock()
				continue
			}
			slog.Info("UDP 转发会话开始", "host", rule.hostname, "target_port", rule.targetPort, "client", key)
			// 回程 goroutine：会话上行数据报 → 回投给该客户端；会话结束或空闲则回收 flow。
			go func(sess *forwardSession, ca net.Addr, key string) {
				defer func() {
					s.forward.removeSession(sess.id)
					mu.Lock()
					delete(flows, key)
					mu.Unlock()
				}()
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case b := <-sess.toUser:
						sess.touch()
						_, _ = pc.WriteTo(b, ca)
					case <-ticker.C:
						sess.mu.Lock()
						idle := time.Now().Unix() - sess.lastActive
						sess.mu.Unlock()
						if idle > int64(forwardUDPFlowIdle/time.Second) {
							sess.closeWith(Tz("log.forward_reason_timeout"))
							return
						}
					case <-sess.done:
						return
					}
				}
			}(sess, clientAddr, key)
		}
		sess.touch()
		select {
		case sess.toAgent <- forwardFrame('d', data):
		case <-sess.done:
		}
	}
}

// readForwardTxFrames 解析 agent 上行的分帧数据报（[type:1][len:2 BE][payload]）。
// 每个 'd' 帧 = 一个 UDP 数据报，投递到会话的 toUser 供回程 goroutine 回投客户端。
// UDP 会话的 tx 走此路径；TCP/HTTP 会话仍走 handleAgentForwardTx 的裸字节流路径。
func readForwardTxFrames(r io.Reader, sess *forwardSession) {
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
		if hdr[0] == 'd' {
			sess.touch()
			select {
			case sess.toUser <- payload:
			case <-sess.done:
				return
			}
		}
	}
}
