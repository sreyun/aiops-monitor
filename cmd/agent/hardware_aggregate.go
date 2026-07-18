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
//
// v6.15.0: map key 从 TargetName 改为 TargetURL。用户在 config.json 中改名后，
// TargetURL（BMC/存储控制器地址）不变，按 URL 去重可确保改名后的快照覆盖旧条目，
// 而非并存。服务端同时做 target_url 迁移保证 PG 历史记录连续。
type hardwareAggregator struct {
	hostID string
	fp     string
	post   func(shared.HardwareReport)

	mu       sync.Mutex
	byTarget map[string]shared.HardwareSnapshot // key = TargetURL (stable across renames)
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
		// 按 TargetURL 去重：改名不改 URL，同一物理设备的快照始终覆盖同一条目。
		// TargetURL 为空时回落到 TargetName（理论上不会发生，采集器始终填 URL）。
		key := s.TargetURL
		if key == "" {
			key = s.TargetName
		}
		a.byTarget[key] = s
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
