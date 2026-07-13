package main

import "testing"

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
