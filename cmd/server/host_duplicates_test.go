package main

import (
	"testing"
	"time"
)

// 清理是**不可逆**的，所以判据必须严到不可能误伤。
// 这些测试守的是"什么情况下绝不能删"。

func dupTestServer(t *testing.T, hosts ...*Host) *Server {
	t.Helper()
	s := &Server{store: NewStore(), cfg: newTestConfigStore(t)}
	for _, h := range hosts {
		s.store.mu.Lock()
		s.store.hosts[h.ID] = h
		s.store.mu.Unlock()
	}
	return s
}

func TestFindDuplicatesGroupsByFingerprint(t *testing.T) {
	now := time.Now().Unix()
	s := dupTestServer(t,
		// 同一台物理机：重装后老 id 离线、新 id 在线
		&Host{ID: "old-id", Hostname: "web01", Fingerprint: "fp-A", LastSeen: now - 86400},
		&Host{ID: "new-id", Hostname: "web01", Fingerprint: "fp-A", LastSeen: now},
		// 另一台机器，同名但指纹不同 —— **绝不能**被并进上面那组
		&Host{ID: "other", Hostname: "web01", Fingerprint: "fp-B", LastSeen: now},
	)

	groups := s.findDuplicateHosts()
	if len(groups) != 1 {
		t.Fatalf("重复组数 = %d, want 1（同名但指纹不同的机器不是重复）", len(groups))
	}
	g := groups[0]
	if len(g.Hosts) != 2 {
		t.Fatalf("组内主机数 = %d, want 2", len(g.Hosts))
	}
	// 最近上报的是当前身份
	if !g.Hosts[0].Current || g.Hosts[0].ID != "new-id" {
		t.Errorf("当前身份判定错误: %+v", g.Hosts[0])
	}
	if g.Hosts[1].Current || g.Hosts[1].ID != "old-id" {
		t.Errorf("老记录不应是 current: %+v", g.Hosts[1])
	}
	if !g.Hosts[1].Stale || g.Stale != 1 {
		t.Errorf("老记录应可清理: %+v stale=%d", g.Hosts[1], g.Stale)
	}
}

// 两条都还在上报 = 很可能是克隆了状态文件的两台真机，谁都不能删。
func TestFindDuplicatesNeverStaleWhenBothOnline(t *testing.T) {
	now := time.Now().Unix()
	s := dupTestServer(t,
		&Host{ID: "a", Hostname: "web01", Fingerprint: "fp-A", LastSeen: now},
		&Host{ID: "b", Hostname: "web01", Fingerprint: "fp-A", LastSeen: now - 1},
	)
	groups := s.findDuplicateHosts()
	if len(groups) != 1 {
		t.Fatalf("组数 = %d", len(groups))
	}
	if groups[0].Stale != 0 {
		t.Errorf("两条都在线时不应有可清理项，实际 %d —— 会误删正在跑的机器", groups[0].Stale)
	}
}

// 没绑定指纹的记录无法可靠判定，宁可不报也不要误删。
func TestFindDuplicatesIgnoresEmptyFingerprint(t *testing.T) {
	now := time.Now().Unix()
	s := dupTestServer(t,
		&Host{ID: "a", Hostname: "web01", Fingerprint: "", LastSeen: now - 86400},
		&Host{ID: "b", Hostname: "web01", Fingerprint: "", LastSeen: now},
	)
	if got := s.findDuplicateHosts(); len(got) != 0 {
		t.Errorf("无指纹主机不应判为重复: %+v", got)
	}
}

func TestFindDuplicatesSingleHostIsNotDuplicate(t *testing.T) {
	s := dupTestServer(t, &Host{ID: "a", Hostname: "web01", Fingerprint: "fp-A", LastSeen: time.Now().Unix()})
	if got := s.findDuplicateHosts(); len(got) != 0 {
		t.Errorf("只有一条记录不算重复: %+v", got)
	}
}

// 清理只动 stale 的那条，当前身份必须活下来。
func TestCleanupDeletesOnlyStale(t *testing.T) {
	now := time.Now().Unix()
	s := dupTestServer(t,
		&Host{ID: "old-id", Hostname: "web01", Fingerprint: "fp-A", LastSeen: now - 86400},
		&Host{ID: "new-id", Hostname: "web01", Fingerprint: "fp-A", LastSeen: now},
	)
	groups := s.findDuplicateHosts()
	var toDelete []string
	for _, g := range groups {
		for _, h := range g.Hosts {
			if h.Stale {
				toDelete = append(toDelete, h.ID)
			}
		}
	}
	if len(toDelete) != 1 || toDelete[0] != "old-id" {
		t.Fatalf("待清理 = %v, want [old-id]", toDelete)
	}
	for _, id := range toDelete {
		s.store.DeleteHost(id)
	}
	if _, ok := s.store.GetHost("new-id"); !ok {
		t.Error("当前身份被误删了")
	}
}
