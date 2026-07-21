package main

import "testing"

// TestReconcileDebounce 验证告警抖动抑制的「触发一次 / 恢复一次」语义：
//   - 候选需连续出现 alertConfirmTicks 次才触发一次，之后持续存在不再重复触发；
//   - 只出现一次（未达确认阈值）就消失的瞬时抖动不产生任何通知；
//   - 已触发告警需连续消失 alertClearTicks 次才恢复一次；
//   - 恢复过程中若在清除阈值内又出现，则不产生恢复通知（抖动被吸收）。
func TestReconcileDebounce(t *testing.T) {
	n := NewNotifier(NewStore(), newTestConfigStore(t))
	a := Alert{HostID: "h1", Type: "cpu", Scope: "", Level: "warning", Message: "cpu high"}
	key := alertKey(a)
	on := map[string]Alert{key: a}
	off := map[string]Alert{}

	// tick 1：候选首次出现 → 尚未确认，不应触发
	if f, r := n.reconcile(on); len(f) != 0 || len(r) != 0 {
		t.Fatalf("tick1 不应触发/恢复，得 fires=%d resolves=%d", len(f), len(r))
	}
	// tick 2：连续第二次出现 → 确认并触发一次
	if f, r := n.reconcile(on); len(f) != 1 || len(r) != 0 {
		t.Fatalf("tick2 应恰好触发一次，得 fires=%d resolves=%d", len(f), len(r))
	}
	// tick 3：持续存在 → 不再重复触发
	if f, r := n.reconcile(on); len(f) != 0 || len(r) != 0 {
		t.Fatalf("tick3 持续告警不应重复触发，得 fires=%d resolves=%d", len(f), len(r))
	}
	// tick 4：消失一次 → 未达清除阈值，不应恢复
	if f, r := n.reconcile(off); len(f) != 0 || len(r) != 0 {
		t.Fatalf("tick4 未达清除阈值不应恢复，得 fires=%d resolves=%d", len(f), len(r))
	}
	// tick 5：连续第二次消失 → 恢复一次
	if f, r := n.reconcile(off); len(f) != 0 || len(r) != 1 {
		t.Fatalf("tick5 应恰好恢复一次，得 fires=%d resolves=%d", len(f), len(r))
	}
	// tick 6：已恢复且仍消失 → 无任何通知
	if f, r := n.reconcile(off); len(f) != 0 || len(r) != 0 {
		t.Fatalf("tick6 不应再有任何通知，得 fires=%d resolves=%d", len(f), len(r))
	}
}

// TestReconcileTransientFlapAbsorbed 验证只闪现一次的瞬时抖动不会触发通知。
func TestReconcileTransientFlapAbsorbed(t *testing.T) {
	n := NewNotifier(NewStore(), newTestConfigStore(t))
	a := Alert{HostID: "h2", Type: "mem", Level: "warning", Message: "mem"}
	on := map[string]Alert{alertKey(a): a}
	off := map[string]Alert{}
	if f, _ := n.reconcile(on); len(f) != 0 { // 出现一次
		t.Fatalf("首次出现不应触发")
	}
	if f, r := n.reconcile(off); len(f) != 0 || len(r) != 0 { // 立刻消失
		t.Fatalf("瞬时抖动应被吸收，得 fires=%d resolves=%d", len(f), len(r))
	}
	// 再次出现应从头计数，仍需两次才触发
	if f, _ := n.reconcile(on); len(f) != 0 {
		t.Fatalf("抖动后再次出现应重新计数，不应立即触发")
	}
	if f, _ := n.reconcile(on); len(f) != 1 {
		t.Fatalf("连续两次出现应触发一次")
	}
}
