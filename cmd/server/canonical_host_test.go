package main

import "testing"

// 规范身份是**治本**修复：Agent 重装会带新的随机 host_id 来，服务端按机器指纹
// 把它认回原来的 id，从而不再产生重复记录，历史（VM 指标 / 日志 / 告警 / 硬件快照）
// 也不会被劈成两半。
//
// 这里守的底线是"绝不能把两台不同的机器并成一台" —— 那比重复记录严重得多：
// 会把两台机器的监控数据混在一起。

func canonStore(t *testing.T, hosts ...*Host) *Store {
	t.Helper()
	s := NewStore()
	for _, h := range hosts {
		s.mu.Lock()
		s.hosts[h.ID] = h
		s.mu.Unlock()
	}
	return s
}

func TestCanonicalHostIDAdoptsOldestOnReinstall(t *testing.T) {
	s := canonStore(t,
		&Host{ID: "orig", Hostname: "web01", Fingerprint: "fp-A", FirstSeen: 1000},
		&Host{ID: "dup", Hostname: "web01", Fingerprint: "fp-A", FirstSeen: 2000},
	)
	// 重装后的全新 id 来注册 → 应认回最早那条
	got, ok := s.CanonicalHostID("brand-new", "fp-A")
	if !ok || got != "orig" {
		t.Fatalf("CanonicalHostID = (%q,%v), want (orig,true)", got, ok)
	}
	// 已经攒下的重复记录也要能自愈：dup 再次注册时同样被指回 orig，
	// 于是历史重新接续，dup 变成可清理的孤儿。
	got, ok = s.CanonicalHostID("dup", "fp-A")
	if !ok || got != "orig" {
		t.Errorf("已存在的重复记录未自愈: got (%q,%v), want (orig,true)", got, ok)
	}
}

func TestCanonicalHostIDNoChangeWhenAlreadyCanonical(t *testing.T) {
	s := canonStore(t, &Host{ID: "orig", Fingerprint: "fp-A", FirstSeen: 1000})
	if got, ok := s.CanonicalHostID("orig", "fp-A"); ok {
		t.Errorf("本来就是规范身份不应改动: got (%q,%v)", got, ok)
	}
}

// 底线：指纹不同 = 不同机器，绝不能合并。
func TestCanonicalHostIDNeverMergesDifferentMachines(t *testing.T) {
	s := canonStore(t, &Host{ID: "web01-a", Hostname: "web01", Fingerprint: "fp-A", FirstSeen: 1000})
	if got, ok := s.CanonicalHostID("web01-b", "fp-B"); ok {
		t.Errorf("不同指纹被误判为同一台机器: got (%q,%v) —— 会把两台机器的数据混在一起", got, ok)
	}
}

// 没有指纹时无从判定，必须保持原样而不是乱认。
func TestCanonicalHostIDRequiresFingerprint(t *testing.T) {
	s := canonStore(t, &Host{ID: "orig", Fingerprint: "", FirstSeen: 1000})
	if got, ok := s.CanonicalHostID("new", ""); ok {
		t.Errorf("空指纹不应匹配任何主机: got (%q,%v)", got, ok)
	}
}

// 全新机器（指纹从没见过）应当拿到自己的新身份。
func TestCanonicalHostIDNewMachineKeepsOwnID(t *testing.T) {
	s := canonStore(t, &Host{ID: "orig", Fingerprint: "fp-A", FirstSeen: 1000})
	if got, ok := s.CanonicalHostID("fresh", "fp-NEW"); ok {
		t.Errorf("全新机器不应被认成已有主机: got (%q,%v)", got, ok)
	}
}

// 运维手工删掉主机后，同一台机器应能以全新身份重新加入（记录已不在 store 里）。
func TestCanonicalHostIDAfterDeleteGivesFreshIdentity(t *testing.T) {
	s := canonStore(t, &Host{ID: "orig", Fingerprint: "fp-A", FirstSeen: 1000})
	s.DeleteHost("orig")
	if got, ok := s.CanonicalHostID("new", "fp-A"); ok {
		t.Errorf("删除后不应再认回旧身份: got (%q,%v)", got, ok)
	}
}
