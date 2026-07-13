package main

import (
	"strings"
	"testing"
)

// redactInlineSecrets 必须把命令行里明文的密码/令牌脱敏，且不误伤普通命令。
func TestRedactInlineSecrets(t *testing.T) {
	secret := []struct{ in, leak string }{
		{"mysql -uroot -pSecret123 db", "Secret123"},
		{"curl -H 'token: abc123xyz'", "abc123xyz"},
		{"export DB_PASSWORD=hunter2", "hunter2"},
		{"echo password=topsecret", "topsecret"},
		{"psql --password verysecret", "verysecret"},
	}
	for _, c := range secret {
		out := redactInlineSecrets(c.in)
		if strings.Contains(out, c.leak) {
			t.Errorf("未脱敏，仍含密钥：%q → %q", c.in, out)
		}
		if !strings.Contains(out, "***") {
			t.Errorf("未见 *** 脱敏标记：%q → %q", c.in, out)
		}
	}
	// 普通命令不得被改动（尤其非 mysql 的 -p 不能误伤）
	for _, ok := range []string{"ls -la /tmp", "tar -pxf a.tar", "grep -rn foo ."} {
		if out := redactInlineSecrets(ok); out != ok {
			t.Errorf("普通命令被误改：%q → %q", ok, out)
		}
	}
}

// 密码提示后紧跟的输入行（即用户键入的密码）绝不能进入命令审计。
func TestPasswordPromptSuppressed(t *testing.T) {
	s := &termSession{}
	s.notePasswordPrompt([]byte("[sudo] password for eason: "))
	if cmd := s.processCommandAudit([]byte("mySudoPass\n")); cmd != "" {
		t.Errorf("sudo 密码被审计了：%q", cmd)
	}
	if cmd := s.processCommandAudit([]byte("whoami\n")); cmd != "whoami" {
		t.Errorf("密码后的普通命令应正常审计，得到 %q", cmd)
	}
	// 中文密码提示同样抑制
	s.notePasswordPrompt([]byte("请输入密码："))
	if cmd := s.processCommandAudit([]byte("zhongwenPass\n")); cmd != "" {
		t.Errorf("中文密码提示后密码被审计：%q", cmd)
	}
}
