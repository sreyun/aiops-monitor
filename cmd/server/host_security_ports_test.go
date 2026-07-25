package main

import (
	"strings"
	"testing"
)

func TestParseListenPortsSS(t *testing.T) {
	lines := []string{
		"Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process",
		`LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:(("sshd",pid=100,fd=3))`,
		`LISTEN 0 128 127.0.0.1:5432 0.0.0.0:* users:(("postgres",pid=200,fd=5))`,
		`LISTEN 0 511 *:6379 *:* users:(("redis-server",pid=300,fd=6))`,
		`LISTEN 0 128 [::]:3306 [::]:* users:(("mysqld",pid=400,fd=7))`,
		`LISTEN 0 128 0.0.0.0:3306 0.0.0.0:* users:(("mysqld",pid=400,fd=8))`, // dual-stack merge
		`ESTAB 0 0 10.0.0.1:22 10.0.0.2:44444 users:(("sshd",pid=101,fd=4))`,
	}
	ports := parseListenPorts(lines)
	if len(ports) != 4 {
		t.Fatalf("want 4 unique proto/port, got %d: %+v", len(ports), ports)
	}
	byPort := map[int]HostOpenPort{}
	for _, p := range ports {
		byPort[p.Port] = p
	}
	if byPort[22].Risk != "medium" || !byPort[22].Public || byPort[22].Service != "ssh" {
		t.Fatalf("ssh: %+v", byPort[22])
	}
	if byPort[6379].Risk != "crit" || !byPort[6379].Public {
		t.Fatalf("redis public: %+v", byPort[6379])
	}
	if byPort[5432].Risk != "medium" || byPort[5432].Public {
		// local bind lowers crit/high → medium
		t.Fatalf("postgres local: %+v", byPort[5432])
	}
	if byPort[3306].Risk != "high" || !byPort[3306].Public {
		t.Fatalf("mysql: %+v", byPort[3306])
	}
}

func TestParseListenPortsDedupDualStack(t *testing.T) {
	lines := []string{
		`sshd 1 root TCP *:5432 (LISTEN)`,
		`postgres 2 eason TCP [::]:5432 (LISTEN)`,
		`redis-ser 3 eason TCP 127.0.0.1:6379 (LISTEN)`,
		`redis-ser 3 eason TCP *:6379 (LISTEN)`,
	}
	ports := parseListenPorts(lines)
	byPort := map[int]HostOpenPort{}
	for _, p := range ports {
		if byPort[p.Port].Port != 0 {
			t.Fatalf("duplicate port %d", p.Port)
		}
		byPort[p.Port] = p
	}
	if len(byPort) != 2 {
		t.Fatalf("got %#v", byPort)
	}
	if byPort[6379].Risk != "crit" || !byPort[6379].Public {
		t.Fatalf("redis should merge to public crit: %+v", byPort[6379])
	}
}

func TestParseListenPortsNetstatDarwin(t *testing.T) {
	lines := []string{
		"tcp4       0      0  *.22                 *.*                LISTEN",
		"tcp4       0      0  127.0.0.1.631        *.*                LISTEN",
		"tcp46      0      0  *.445                *.*                LISTEN",
	}
	ports := parseListenPorts(lines)
	byPort := map[int]HostOpenPort{}
	for _, p := range ports {
		byPort[p.Port] = p
	}
	if byPort[22].Port != 22 || !byPort[22].Public {
		t.Fatalf("ssh: %+v", byPort[22])
	}
	if byPort[445].Risk != "crit" {
		t.Fatalf("smb: %+v", byPort[445])
	}
	if _, ok := byPort[631]; !ok {
		t.Fatal("missing cups 631")
	}
}

func TestParseListenPortsLsof(t *testing.T) {
	lines := []string{
		"COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME",
		"sshd      100 root    3u  IPv4 0x1      0t0  TCP *:22 (LISTEN)",
		"mysqld    200 mysql  30u  IPv6 0x2      0t0  TCP *:3306 (LISTEN)",
		"redis-ser 300 redis   6u  IPv4 0x3      0t0  TCP 127.0.0.1:6379 (LISTEN)",
	}
	ports := parseListenPorts(lines)
	byPort := map[int]HostOpenPort{}
	for _, p := range ports {
		byPort[p.Port] = p
	}
	if byPort[22].Process != "sshd" || byPort[22].Risk != "medium" {
		t.Fatalf("ssh: %+v", byPort[22])
	}
	if byPort[3306].Risk != "high" || byPort[3306].Process != "mysqld" {
		t.Fatalf("mysql: %+v", byPort[3306])
	}
	if byPort[6379].Public || byPort[6379].Risk != "medium" {
		t.Fatalf("local redis should be medium: %+v", byPort[6379])
	}
}

func TestPortRiskFindings(t *testing.T) {
	fs := portRiskFindings([]HostOpenPort{
		{Proto: "tcp", Port: 6379, Addr: "0.0.0.0", Public: true, Risk: "crit", Service: "redis"},
		{Proto: "tcp", Port: 80, Addr: "0.0.0.0", Public: true},
	})
	if len(fs) != 1 || fs[0].Category != "port" || !strings.Contains(fs[0].Title, "6379") {
		t.Fatalf("%+v", fs)
	}
}

func TestSummarizePorts(t *testing.T) {
	n, risky, sample := summarizePorts([]HostOpenPort{
		{Port: 22, Risk: "medium"},
		{Port: 80},
		{Port: 443},
		{Port: 22, Risk: "medium"},
	})
	// risky counted by unique port number
	if n != 4 || risky != 1 {
		t.Fatalf("n=%d risky=%d", n, risky)
	}
	if len(sample) == 0 || sample[0] != 22 && sample[0] != 80 && sample[0] != 443 {
		t.Fatalf("sample=%v", sample)
	}
}
