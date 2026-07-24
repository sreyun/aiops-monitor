package main

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aiops-monitor/shared"
)

func TestParseTSharkCaptureLineTCP(t *testing.T) {
	httpPayload := []byte("POST /v1/chat/completions HTTP/1.1\r\nHost: llm.example\r\n\r\n{}")
	fields := make([]string, len(tsharkFields))
	fields[0] = "1710000000.1"
	fields[1], fields[3] = "10.0.0.2", "10.0.0.8"
	fields[5], fields[6] = "51000", "8000"
	fields[9], fields[10] = "123456", "0x0018"
	fields[11] = hex.EncodeToString(httpPayload)
	rec, ok := parseTSharkCaptureLine(strings.Join(fields, "\t"))
	if !ok || rec.info.proto != 6 || rec.info.srcPort != 51000 || rec.info.dstPort != 8000 {
		t.Fatalf("TCP fields not parsed: %+v ok=%v", rec, ok)
	}
	if rec.info.seq != 123456 || rec.info.tcpFlags != 0x18 || string(rec.info.payload) != string(httpPayload) {
		t.Fatalf("TCP payload metadata not parsed: %+v", rec.info)
	}
}

func TestParseTSharkCaptureLineIPv6DNSAndSNI(t *testing.T) {
	fields := make([]string, len(tsharkFields))
	fields[2], fields[4] = "2001:db8::1", "2001:db8::2"
	fields[5], fields[6] = "52000", "443"
	fields[13] = "api.openai.com"
	rec, ok := parseTSharkCaptureLine(strings.Join(fields, "\t"))
	if !ok || rec.info.dstIP != "2001:db8::2" || rec.sni != "api.openai.com" {
		t.Fatalf("IPv6/SNI fields not parsed: %+v ok=%v", rec, ok)
	}

	fields = make([]string, len(tsharkFields))
	fields[1], fields[3] = "8.8.8.8", "10.0.0.2"
	fields[7], fields[8] = "53", "53000"
	fields[14], fields[16] = "llm.internal.", "2001:db8::8"
	rec, ok = parseTSharkCaptureLine(strings.Join(fields, "\t"))
	if !ok || rec.dnsName != "llm.internal" || rec.dnsAddress != "2001:db8::8" {
		t.Fatalf("DNS fields not parsed: %+v ok=%v", rec, ok)
	}
}

func TestBuildTSharkCaptureFilterIsBounded(t *testing.T) {
	got := buildTSharkCaptureFilter(SNIConfig{
		TLSMetadataPorts: []int{443, 443, 8443},
		ContentAudit:     true, ContentAuditPorts: []int{8000, 11434},
	})
	for _, want := range []string{"udp port 53", "tcp port 443", "tcp port 8000", "tcp port 11434"} {
		if !strings.Contains(got, want) {
			t.Fatalf("filter %q missing %q", got, want)
		}
	}
	if strings.Count(got, "tcp port 443") != 1 {
		t.Fatalf("filter contains duplicate ports: %q", got)
	}
}

func TestRunTSharkDrainsPacketsBeforeProcessExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a temporary POSIX shell fixture")
	}
	payload := "POST /v1/responses HTTP/1.1\r\nHost: llm.example\r\nContent-Length: 2\r\n\r\n{}"
	fields := make([]string, len(tsharkFields))
	fields[1], fields[3] = "10.0.0.1", "10.0.0.2"
	fields[5], fields[6] = "50000", "8000"
	fields[9], fields[10], fields[11] = "100", "0x0019", hex.EncodeToString([]byte(payload))
	line := strings.Join(fields, "\t")
	binary := filepath.Join(t.TempDir(), "tshark")
	script := "#!/bin/sh\nprintf '%s\\n' '" + line + "'\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	sc := newSNICollector(SNIConfig{
		TSharkPath: binary, ContentAudit: true,
		ContentAuditPorts: []int{8000}, ContentAuditBodyMode: "metadata",
	}, "host-1", "fp")
	var reports []shared.ContentAuditReport
	err := sc.runTShark(context.Background(), func(shared.DNSMapReport) {},
		func(rep shared.ContentAuditReport) { reports = append(reports, rep) })
	if err == nil {
		t.Fatal("short-lived tshark fixture should report process exit")
	}
	if len(reports) != 1 || len(reports[0].Events) != 1 {
		t.Fatalf("queued packets were lost on process exit: reports=%+v", reports)
	}
	ev := reports[0].Events[0]
	if ev.Path != "/v1/responses" || ev.BodyMode != "metadata" || ev.ReqBytes != 2 || ev.Body != "" {
		t.Fatalf("captured event policy mismatch: %+v", ev)
	}
}
