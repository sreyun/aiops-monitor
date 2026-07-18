package main

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// 用 RFC 3414 附录 A.3 的官方测试向量验证口令派生 + 密钥本地化。

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func TestPasswordToKeyMD5_RFC3414(t *testing.T) {
	// A.3.1: password "maplesyrup"
	ku := passwordToKey("MD5", []byte("maplesyrup"))
	want := mustHex("9faf3283884e92834ebc9847d8edd963")
	if !bytes.Equal(ku, want) {
		t.Fatalf("MD5 Ku = %x, 期望 %x", ku, want)
	}
	engineID := mustHex("000000000000000000000002")
	kul := localizeKey("MD5", ku, engineID)
	wantKul := mustHex("526f5eed9fcce26f8964c2930787d82b")
	if !bytes.Equal(kul, wantKul) {
		t.Fatalf("MD5 Kul = %x, 期望 %x", kul, wantKul)
	}
}

func TestPasswordToKeySHA_RFC3414(t *testing.T) {
	// A.3.2: password "maplesyrup"
	ku := passwordToKey("SHA", []byte("maplesyrup"))
	want := mustHex("9fb5cc0381497b3793528939ff788d5d79145211")
	if !bytes.Equal(ku, want) {
		t.Fatalf("SHA Ku = %x, 期望 %x", ku, want)
	}
	// Kul 是 RFC 3414 A.3.2 权威值（端到端锚点，独立校验 python 一致）。
	engineID := mustHex("000000000000000000000002")
	kul := localizeKey("SHA", ku, engineID)
	wantKul := mustHex("6695febc9288e36282235fc7151f128497b38f3f")
	if !bytes.Equal(kul, wantKul) {
		t.Fatalf("SHA Kul = %x, 期望 %x", kul, wantKul)
	}
}

func TestAuthParamLen(t *testing.T) {
	if authParamLen("MD5") != 12 || authParamLen("SHA") != 12 || authParamLen("SHA256") != 24 {
		t.Error("authParamLen 错")
	}
}

func TestDESRoundTrip(t *testing.T) {
	privKul := mustHex("526f5eed9fcce26f8964c2930787d82b") // 16 字节
	pt := []byte("hello snmp v3 scoped pdu payload!!")     // 非 8 整数倍，触发补位
	ct, salt, err := encryptDES(privKul, 5, 0xdeadbeef, pt)
	if err != nil {
		t.Fatalf("encryptDES: %v", err)
	}
	if len(salt) != 8 {
		t.Fatalf("DES salt 长度 %d", len(salt))
	}
	dec, err := decryptDES(privKul, salt, ct)
	if err != nil {
		t.Fatalf("decryptDES: %v", err)
	}
	if !bytes.Equal(dec[:len(pt)], pt) {
		t.Errorf("DES 往返失败: %q", dec[:len(pt)])
	}
}

func TestAESRoundTrip(t *testing.T) {
	privKul := mustHex("526f5eed9fcce26f8964c2930787d82b")
	pt := []byte("hello snmp v3 aes cfb payload of arbitrary length")
	ct, salt, err := encryptAES(privKul, 7, 123456, 0x0102030405060708, pt)
	if err != nil {
		t.Fatalf("encryptAES: %v", err)
	}
	if len(ct) != len(pt) { // CFB 流模式，长度不变
		t.Fatalf("AES 密文长度 %d != 明文 %d", len(ct), len(pt))
	}
	dec, err := decryptAES(privKul, 7, 123456, salt, ct)
	if err != nil {
		t.Fatalf("decryptAES: %v", err)
	}
	if !bytes.Equal(dec, pt) {
		t.Errorf("AES 往返失败: %q", dec)
	}
}

func TestDeriveSecLevel(t *testing.T) {
	cases := []struct {
		explicit string
		auth     string
		priv     string
		want     int
	}{
		{"", "", "", 0},
		{"", "SHA", "", 1},
		{"", "SHA", "AES", 3},
		{"authPriv", "", "", 3}, // 显式优先
		{"noAuthNoPriv", "SHA", "AES", 0},
	}
	for _, c := range cases {
		u := &usmUser{authProto: normalizeAuthProto(c.auth), privProto: normalizePrivProto(c.priv)}
		if got := deriveSecLevel(c.explicit, u); got != c.want {
			t.Errorf("deriveSecLevel(%q,%q,%q) = %d, 期望 %d", c.explicit, c.auth, c.priv, got, c.want)
		}
	}
}

// 验证 authPriv 报文能编排出来且 HMAC 原位写入不破坏结构（能被 parse 回去）。
func TestBuildV3MessageAuthPriv(t *testing.T) {
	u := &usmUser{
		name: "monitor", secLevel: 3,
		authProto: "SHA", authPass: []byte("authpassword"),
		privProto: "AES", privPass: []byte("privpassword"),
	}
	e := &engineEntry{id: mustHex("80001f8880")}
	e.deriveKeys(u)
	msg, err := u.buildV3Message(e, 42, buildGet(99, [][]uint32{oidSysDescr}))
	if err != nil {
		t.Fatalf("buildV3Message: %v", err)
	}
	// 能定位到 authParams 且长度=12（SHA-96），说明结构正确、HMAC 已就位
	authP, err := locateAuthParams(msg)
	if err != nil {
		t.Fatalf("locateAuthParams: %v", err)
	}
	if len(authP) != 12 {
		t.Errorf("authParams 长度 %d, 期望 12", len(authP))
	}
	// authParams 不应全 0（HMAC 已写入）
	allZero := true
	for _, b := range authP {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("authParams 仍全 0，HMAC 未写入")
	}
	// 外层能被 parse（secparams 可抽取）
	if _, _, err := parseV3SecParams(msg); err != nil {
		t.Errorf("parseV3SecParams 失败: %v", err)
	}
}
