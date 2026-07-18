package shared

import (
	"reflect"
	"testing"
)

type fmtCfg struct {
	Server   string   `json:"server"`
	Interval int      `json:"report_interval"`
	Protos   []string `json:"protocols"`
	SNMP     struct {
		Listen string `json:"listen"`
		Users  []struct {
			User     string `json:"user"`
			SecLevel string `json:"sec_level"`
		} `json:"trap_users"`
	} `json:"snmp"`
}

// TestDecodeConfigJSONvsYAML 确认同一份配置的 JSON 与 YAML 表达解析出完全一致的结构，
// 且复用了 `json:` tag（YAML 无需额外 yaml tag）。
func TestDecodeConfigJSONvsYAML(t *testing.T) {
	jsonData := []byte(`{
		"server":"http://x:8529","report_interval":30,"protocols":["v5","v9"],
		"snmp":{"listen":":162","trap_users":[{"user":"u1","sec_level":"authPriv"}]}
	}`)
	yamlData := []byte("" +
		"server: http://x:8529\n" +
		"report_interval: 30\n" +
		"protocols:\n  - v5\n  - v9\n" +
		"snmp:\n" +
		"  listen: \":162\"\n" +
		"  trap_users:\n" +
		"    - user: u1\n" +
		"      sec_level: authPriv\n")

	var cj, cy fmtCfg
	if err := DecodeConfig("config.json", jsonData, &cj); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if err := DecodeConfig("config.yaml", yamlData, &cy); err != nil {
		t.Fatalf("YAML 解析失败: %v", err)
	}
	if !reflect.DeepEqual(cj, cy) {
		t.Fatalf("JSON 与 YAML 解析结果不一致:\n json=%+v\n yaml=%+v", cj, cy)
	}
	if cy.Server != "http://x:8529" || cy.Interval != 30 || cy.SNMP.Listen != ":162" ||
		len(cy.Protos) != 2 || len(cy.SNMP.Users) != 1 || cy.SNMP.Users[0].SecLevel != "authPriv" {
		t.Fatalf("YAML 字段解析错: %+v", cy)
	}

	// .yml 也按 YAML 解析
	var cyml fmtCfg
	if err := DecodeConfig("config.yml", yamlData, &cyml); err != nil || cyml.Interval != 30 {
		t.Fatalf(".yml 应按 YAML 解析: %+v err=%v", cyml, err)
	}

	// 无扩展名/其它扩展名 → 按 JSON 解析
	if IsYAMLPath("config.txt") || !IsYAMLPath("a.YAML") || !IsYAMLPath("b.yml") {
		t.Fatal("IsYAMLPath 扩展名判断错")
	}
}
