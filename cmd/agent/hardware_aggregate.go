package main

import (
	"sort"
	"sync"

	"aiops-monitor/shared"
)

// hardwareAggregator merges HardwareReports from every hardware collector
// (Redfish BMC, OceanStor DeviceManager, …) into one report per host.
//
// 为什么必须合并：服务端 hardwareStore.put 用整份快照集合**替换**该主机的记录，
// pgstore 也按 (host_id, target_name) upsert。若两个采集器各自 POST 自己那份
// 列表，内存里的告警评估集合就会被后到的一方整体覆盖，导致另一方的所有硬件
// 告警每轮 fire→resolve→fire 抖动。按 target 合并后再上报即可根除。
type hardwareAggregator struct {
	hostID string
	fp     string
	post   func(shared.HardwareReport)

	mu       sync.Mutex
	byTarget map[string]shared.HardwareSnapshot
}

func newHardwareAggregator(hostID, fp string, post func(shared.HardwareReport)) *hardwareAggregator {
	return &hardwareAggregator{
		hostID:   hostID,
		fp:       fp,
		post:     post,
		byTarget: make(map[string]shared.HardwareSnapshot),
	}
}

// submit merges one collector's snapshots and posts the union of all collectors'.
func (a *hardwareAggregator) submit(rep shared.HardwareReport) {
	a.mu.Lock()
	for _, s := range rep.Snapshots {
		// 与服务端 upsert 的主键保持一致（host_id, target_name）
		a.byTarget[s.TargetName] = s
	}
	all := make([]shared.HardwareSnapshot, 0, len(a.byTarget))
	for _, s := range a.byTarget {
		all = append(all, s)
	}
	a.mu.Unlock()

	// map 迭代顺序随机，排一下让上报内容稳定（便于比对与排错）
	sort.Slice(all, func(i, j int) bool { return all[i].TargetName < all[j].TargetName })
	a.post(shared.HardwareReport{HostID: a.hostID, Fingerprint: a.fp, Snapshots: all})
}
