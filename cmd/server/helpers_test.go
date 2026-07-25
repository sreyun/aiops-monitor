package main

import (
	"strings"
	"testing"
)

// smsSafeVar 必须剔除阿里云短信不支持的内容（emoji/换行/【】等），否则 isv.UNSUPPORTED_SMS_CONTENT。
func TestSMSSafeVar(t *testing.T) {
	out := smsSafeVar("【AIOps】测试消息：告警通道连通正常 ✅\n时间: 2026-07-13 19:20:30")
	if strings.ContainsAny(out, "✅【】\n\r\t") {
		t.Errorf("清洗后仍含 emoji/换行/括号：%q", out)
	}
	if !strings.Contains(out, "AIOps") || !strings.Contains(out, "测试消息") {
		t.Errorf("正常内容被误删：%q", out)
	}
	if len([]rune(out)) > smsSafeVarMax {
		t.Errorf("未按 %d 字截断：%d 字 %q", smsSafeVarMax, len([]rune(out)), out)
	}
}

// 长告警正文（主机+IP+异常详情）不得被旧的 45 字上限截断。
func TestSMSSafeVarKeepsHostIPDetail(t *testing.T) {
	msg := "AIOps触发 严重 主机web-prod-01 IP10.20.30.40 类型CPU 详情CPU使用率96.2%（空闲3.8%）超过阈值95% 时间2026-07-25 13:00:00"
	out := smsSafeVar(msg)
	if !strings.Contains(out, "web-prod-01") {
		t.Fatalf("主机名被截断/丢失：%q", out)
	}
	if !strings.Contains(out, "10.20.30.40") {
		t.Fatalf("IP 被截断/丢失：%q", out)
	}
	if !strings.Contains(out, "96.2") {
		t.Fatalf("异常详情被截断/丢失：%q", out)
	}
}

func TestFormatAlertSMSIncludesHostIP(t *testing.T) {
	a := Alert{
		HostID:    "h1",
		Hostname:  "web-prod-01",
		IP:        "10.20.30.40",
		Level:     "critical",
		Type:      "cpu",
		Message:   "CPU使用率96.2%（空闲3.8%）",
		Timestamp: 1721880000,
	}
	out := formatAlertSMS(a, true)
	for _, want := range []string{"AIOps", "web-prod-01", "10.20.30.40", "96.2", "IP:10.20.30.40"} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatAlertSMS missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "\n") {
		t.Fatalf("SMS body must be single line: %q", out)
	}
	if !strings.Contains(out, ":web-prod-01") {
		t.Fatalf("formatAlertSMS should separate label and host with colon: %q", out)
	}
}

// mergeSecrets 必须还原 GET 中被脱敏的 短信/语音 AccessKey + SecretKey，否则「发送测试」或
// 再次保存会拿脱敏串（如 LTAI****GHIJ）当真实凭证做 ACS3-HMAC-SHA256 签名 →
// 阿里云返回 SignatureDoesNotMatch / InvalidAccessKeyId。此测试锁定该修复。
func TestMergeSecretsPreservesSMSAndVoiceKeys(t *testing.T) {
	var old ServerConfig
	old.SMS.AccessKey = "LTAI5tRealAccessKeyId"
	old.SMS.SecretKey = "RealSecretKeyValue123456"
	old.VoiceCall.AccessKey = "LTAI5tVoiceAccessKey"
	old.VoiceCall.SecretKey = "VoiceSecretKeyValue123"

	// 表单回传脱敏值（maskSecret 形态：前4 + **** + 后4）
	var in ServerConfig
	in.SMS.AccessKey = maskSecret(old.SMS.AccessKey)
	in.SMS.SecretKey = maskSecret(old.SMS.SecretKey)
	in.VoiceCall.AccessKey = maskSecret(old.VoiceCall.AccessKey)
	in.VoiceCall.SecretKey = maskSecret(old.VoiceCall.SecretKey)

	mergeSecrets(&in, old)

	if in.SMS.AccessKey != old.SMS.AccessKey {
		t.Errorf("SMS.AccessKey 未还原：%q", in.SMS.AccessKey)
	}
	if in.SMS.SecretKey != old.SMS.SecretKey {
		t.Errorf("SMS.SecretKey 未还原：%q", in.SMS.SecretKey)
	}
	if in.VoiceCall.AccessKey != old.VoiceCall.AccessKey {
		t.Errorf("VoiceCall.AccessKey 未还原：%q", in.VoiceCall.AccessKey)
	}
	if in.VoiceCall.SecretKey != old.VoiceCall.SecretKey {
		t.Errorf("VoiceCall.SecretKey 未还原：%q", in.VoiceCall.SecretKey)
	}

	// 真实新值应原样保留（不被 old 覆盖）
	var in2 ServerConfig
	in2.SMS.AccessKey = "LTAI5tNewKeyId"
	in2.SMS.SecretKey = "NewSecretValue"
	mergeSecrets(&in2, old)
	if in2.SMS.AccessKey != "LTAI5tNewKeyId" || in2.SMS.SecretKey != "NewSecretValue" {
		t.Errorf("真实新值被误覆盖：ak=%q sk=%q", in2.SMS.AccessKey, in2.SMS.SecretKey)
	}
}
