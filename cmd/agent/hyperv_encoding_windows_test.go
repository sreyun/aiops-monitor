//go:build windows

package main

import (
	"testing"
)

func TestEnsureUTF8HyperVGBKFields(t *testing.T) {
	// GBK for: {"IntegrationState":"正常","Name":"网络适配器"}
	// 正常 = D5 FD B3 A3 ; 网络适配器 = CD F8 C2 E7 CA CA C5 E4 C6 F7
	gbkJSON := []byte("{\"IntegrationState\":\"\xd5\xfd\xb3\xa3\",\"Nics\":{\"Name\":\"\xcd\xf8\xc2\xe7\xca\xca\xc5\xe4\xc6\xf7\",\"MAC\":\"00:15:5D:01:02:03\",\"Switch\":\"transfer\",\"Status\":\"Ok\",\"Connected\":true,\"IP\":\"1.2.3.4\"},\"Name\":\"vm1\",\"State\":\"Running\"}")
	decoded := string(ensureUTF8(gbkJSON))
	g, err := parseHyperV(decoded)
	if err != nil || len(g) != 1 {
		t.Fatalf("parse after ensureUTF8: err=%v guests=%v decoded=%q", err, g, decoded)
	}
	if g[0].IntegrationState != "正常" {
		t.Fatalf("IntegrationState=%q want 正常 (decoded=%q)", g[0].IntegrationState, decoded)
	}
	if len(g[0].Nics) != 1 || g[0].Nics[0].Name != "网络适配器" {
		t.Fatalf("Nic.Name=%q want 网络适配器", g[0].Nics[0].Name)
	}
}
