package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRewriteAgentConfigYAMLForRelay(t *testing.T) {
	in := `# comment
server: "https://cloud.example:8529"
token: "t"
category: "prod"
`
	out := rewriteAgentConfigYAMLForRelay(in, "http://192.168.1.10:8529")
	if !strings.Contains(out, `server: "http://192.168.1.10:8529"`) {
		t.Fatalf("server not rewritten:\n%s", out)
	}
	if strings.Contains(out, "cloud.example") {
		t.Fatalf("cloud URL still present:\n%s", out)
	}
}

func TestRewriteAgentConfigYAMLDropsServers(t *testing.T) {
	in := `servers: [{"server":"https://a","token":"x"},{"server":"https://b","token":"y"}]
category: "x"
`
	out := rewriteAgentConfigYAMLForRelay(in, "http://gw:8529")
	if strings.Contains(out, "servers:") {
		t.Fatalf("servers block should be removed:\n%s", out)
	}
	if !strings.Contains(out, `server: "http://gw:8529"`) {
		t.Fatalf("missing server:\n%s", out)
	}
}

func TestRewriteInstallScriptForRelayConfigB64(t *testing.T) {
	yaml := "server: \"https://cloud:8529\"\ntoken: \"tok\"\n"
	b64 := base64.StdEncoding.EncodeToString([]byte(yaml))
	script := "SERVER=\"https://cloud:8529\"\nAIOPS_CONFIG_B64='" + b64 + "'\necho ok\n"
	out := rewriteInstallScriptForRelay(script, "http://10.0.0.1:8529")
	if !strings.Contains(out, `SERVER="http://10.0.0.1:8529"`) {
		t.Fatalf("SERVER not rewritten:\n%s", out)
	}
	sub := configB64Re.FindStringSubmatch(out)
	if len(sub) != 4 {
		t.Fatal("CONFIG_B64 missing after rewrite")
	}
	raw, err := base64.StdEncoding.DecodeString(sub[2])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `server: "http://10.0.0.1:8529"`) {
		t.Fatalf("config yaml not rewritten: %s", raw)
	}
	if strings.Contains(string(raw), "cloud") {
		t.Fatalf("cloud still in config: %s", raw)
	}
}
