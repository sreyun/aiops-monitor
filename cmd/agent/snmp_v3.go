package main

// SNMP v3 USM（占位）。完整实现（engine discovery / 口令派生+本地化 / HMAC 认证 /
// DES·AES 加解密 / 报文编排）在阶段 5 补齐；此处先给一个 stub 让 v2c 全链路先编译通过。

import (
	"errors"
	"net"
	"time"
)

// newV3Exchanger 构造一个 SNMPv3 exchanger（阶段 5 实现）。
func newV3Exchanger(conn net.Conn, t SNMPTarget, timeout time.Duration, retries int) (exchanger, error) {
	if conn != nil {
		_ = conn.Close()
	}
	_ = timeout
	_ = retries
	_ = t
	return nil, errors.New("SNMP v3 尚未实现（阶段 5）")
}
