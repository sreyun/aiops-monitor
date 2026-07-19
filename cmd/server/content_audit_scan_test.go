package main

import "testing"

func TestScanSensitive(t *testing.T) {
	if hits := scanSensitive(`{"api_key":"sk-abcdefghijklmnopqrstuvwxyz123456"}`, nil); len(hits) == 0 {
		t.Error("应命中 LLM 密钥/凭据字段")
	}
	if hits := scanSensitive("我的身份证是 11010119900307391X 请记住", nil); len(hits) == 0 {
		t.Error("应命中身份证号")
	}
	if hits := scanSensitive("联系电话 13800138000 谢谢", nil); len(hits) == 0 {
		t.Error("应命中手机号")
	}
	if hits := scanSensitive("这是公司绝密项目文档", []string{"绝密"}); len(hits) == 0 {
		t.Error("应命中自定义关键词")
	}
	if hits := scanSensitive("hello world 普通闲聊没有敏感信息", nil); len(hits) != 0 {
		t.Errorf("普通文本不应命中: %v", hits)
	}
	if sensitiveSeverity([]string{"LLM/OpenAI 密钥"}) != "critical" {
		t.Error("密钥外泄应 critical")
	}
	if sensitiveSeverity([]string{"手机号"}) != "warning" {
		t.Error("PII 手机号应 warning")
	}
}

func TestShouldAlertContent(t *testing.T) {
	if !shouldAlertContent("kA", 1000) {
		t.Error("首次应告警")
	}
	if shouldAlertContent("kA", 1100) {
		t.Error("窗口内同键应去重(不告警)")
	}
	if !shouldAlertContent("kA", 1000+contentAlertWindowSec+1) {
		t.Error("过窗后应可再告警")
	}
	if !shouldAlertContent("kB", 1100) {
		t.Error("不同键应各自告警")
	}
}
