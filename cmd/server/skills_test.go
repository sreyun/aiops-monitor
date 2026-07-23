package main

import (
	"strings"
	"testing"
)

func TestParseDistilledSkillsArray(t *testing.T) {
	raw := "以下是结果：\n```json\n[{\"name\":\"清理日志\",\"trigger\":\"磁盘将满\",\"steps\":\"1. du\\n2. truncate\",\"tags\":\"disk\"}]\n```\n"
	got := parseDistilledSkills(raw)
	if len(got) != 1 || got[0].Name != "清理日志" || !strings.Contains(got[0].Steps, "du") {
		t.Fatalf("got=%+v", got)
	}
}

func TestParseDistilledSkillsSingleObject(t *testing.T) {
	raw := `{"name":"重启卡死服务","trigger":"端口无响应","steps":"systemctl restart app","tags":"service"}`
	got := parseDistilledSkills(raw)
	if len(got) != 1 || got[0].Name != "重启卡死服务" {
		t.Fatalf("got=%+v", got)
	}
}

func TestParseDistilledSkillsEmpty(t *testing.T) {
	if got := parseDistilledSkills("[]"); len(got) != 0 {
		t.Fatalf("want empty, got %+v", got)
	}
	if got := parseDistilledSkills("nonsense"); got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

func TestPromoteTextToSkillSyncRequiresAI(t *testing.T) {
	s := &Server{cfg: &ConfigStore{}}
	created, updated, err := s.promoteTextToSkillSync("resolution", "incident:1", "CPU 过高已解决")
	if err == nil {
		t.Fatal("expected AI 未配置 error")
	}
	if created || updated {
		t.Fatalf("created=%v updated=%v", created, updated)
	}
}
