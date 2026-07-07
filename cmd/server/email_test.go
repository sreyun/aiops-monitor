package main

import "testing"

func TestVerifyCodeSingleUse(t *testing.T) {
	em := newEmailManager()
	code, _ := em.issueCode("a@b.com", "reset_password")
	if !em.verifyCode("a@b.com", "reset_password", code) {
		t.Fatal("正确验证码应通过")
	}
	if em.verifyCode("a@b.com", "reset_password", code) {
		t.Fatal("验证码应单次使用（第二次应失败）")
	}
}

func TestVerifyCodeWrongPurpose(t *testing.T) {
	em := newEmailManager()
	code, _ := em.issueCode("a@b.com", "reset_password")
	if em.verifyCode("a@b.com", "mfa_unbind", code) {
		t.Fatal("purpose 不匹配应失败")
	}
}

// TestVerifyCodeBruteForceCap is the security regression test: a 6-digit code
// must be voided after a small number of wrong guesses so it can't be enumerated.
func TestVerifyCodeBruteForceCap(t *testing.T) {
	em := newEmailManager()
	code, _ := em.issueCode("a@b.com", "reset_password")
	for i := 0; i < 5; i++ {
		em.verifyCode("a@b.com", "reset_password", "abcdef") // 6 chars, never a numeric code
	}
	if em.verifyCode("a@b.com", "reset_password", code) {
		t.Fatal("连续 5 次错误后，即使正确验证码也应作废（防暴破）")
	}
}
