package main

import (
	"sort"
	"strings"

	"aiops-monitor/shared"
)

// histJoinCell is one timestamp while reassembling VM multi-series export /
// query_range into Sample rows. "set" tracks which scalar fields were explicitly
// written so we can LOCF gaps without treating real zeros as "missing".
// gpuFields/diskFields track which nested fields arrived so hold-forward does
// not clobber unset temp/mem/used with Go zeros when only util/percent arrives.
type histJoinCell struct {
	s          shared.Sample
	set        map[string]bool
	gpuFields  map[string]map[string]bool // gpu name -> field short name
	diskFields map[string]map[string]bool // path -> field short name
	connSet    bool
}

func (c *histJoinCell) mark(short string) {
	if c.set == nil {
		c.set = map[string]bool{}
	}
	c.set[short] = true
}

func (c *histJoinCell) marked(short string) bool {
	return c != nil && c.set != nil && c.set[short]
}

func (c *histJoinCell) markGPUField(name, field string) {
	if c.gpuFields == nil {
		c.gpuFields = map[string]map[string]bool{}
	}
	if c.gpuFields[name] == nil {
		c.gpuFields[name] = map[string]bool{}
	}
	c.gpuFields[name][field] = true
}

func (c *histJoinCell) markDiskField(path, field string) {
	if c.diskFields == nil {
		c.diskFields = map[string]map[string]bool{}
	}
	if c.diskFields[path] == nil {
		c.diskFields[path] = map[string]bool{}
	}
	c.diskFields[path][field] = true
}

func ensureHistCell(byTs map[int64]*histJoinCell, ts int64) *histJoinCell {
	c := byTs[ts]
	if c == nil {
		c = &histJoinCell{s: shared.Sample{Timestamp: ts}}
		byTs[ts] = c
	}
	return c
}

// applyHistJoinMetric writes one labeled VM series point into the join map.
func applyHistJoinMetric(byTs map[int64]*histJoinCell, ts int64, name, gpuName, diskPath, connProto, connState string, val float64) {
	c := ensureHistCell(byTs, ts)
	switch {
	case strings.HasPrefix(name, "aiops_gpu_"):
		field := setSampleGPU(&c.s, gpuName, name, val)
		if gpuName == "" {
			gpuName = "GPU"
		}
		if field != "" {
			c.markGPUField(gpuName, field)
		}
	case strings.HasPrefix(name, "aiops_disk_vol_"):
		field := setSampleDisk(&c.s, diskPath, name, val)
		if field != "" && diskPath != "" {
			c.markDiskField(diskPath, field)
		}
	case name == "aiops_net_conn_count":
		setSampleConn(&c.s, connProto, connState, val)
		c.connSet = true
	default:
		short := strings.TrimPrefix(name, "aiops_")
		setSampleMetric(&c.s, name, val)
		c.mark(short)
	}
}

// finalizeHistJoin sorts cells by timestamp, carries gauge fields / nested
// inventories forward across gaps, and returns the Sample slice for the API.
//
// Why: query_range / export emit independent Prom series. Joining by exact ts
// leaves unset Go zeros on samples where only some metrics arrived — charts then
// flicker between 1/2/3 load curves (and forecast drops sparse series).
func finalizeHistJoin(byTs map[int64]*histJoinCell) []shared.Sample {
	if len(byTs) == 0 {
		return nil
	}
	cells := make([]*histJoinCell, 0, len(byTs))
	for _, c := range byTs {
		cells = append(cells, c)
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].s.Timestamp < cells[j].s.Timestamp })

	var (
		lastLoad1, lastLoad5, lastLoad15     *float64
		lastCPU, lastMem, lastDisk, lastSwap *float64
		lastCPUCores                         *int
		lastMemUsed, lastMemTotal            *uint64
		lastSwapUsed, lastSwapTotal          *uint64
		lastDiskUsed, lastDiskTotal          *uint64
		lastUptime                           *uint64
		lastDiskIO, lastDiskRR, lastDiskWR   *float64
		lastDiskRI, lastDiskWI               *float64
		lastNetS, lastNetR                   *float64
		lastNetConns, lastProc               *int
		lastGPUs                             []shared.GPUInfo
		lastDisks                            []shared.DiskInfo
		lastConns                            []shared.ConnStat
	)

	out := make([]shared.Sample, 0, len(cells))
	for _, c := range cells {
		locfF := func(key string, dst *float64, last **float64) {
			if c.marked(key) {
				v := *dst
				*last = &v
				return
			}
			if *last != nil {
				*dst = **last
			}
		}
		locfI := func(key string, dst *int, last **int) {
			if c.marked(key) {
				v := *dst
				*last = &v
				return
			}
			if *last != nil {
				*dst = **last
			}
		}
		locfU := func(key string, dst *uint64, last **uint64) {
			if c.marked(key) {
				v := *dst
				*last = &v
				return
			}
			if *last != nil {
				*dst = **last
			}
		}

		locfF("cpu_percent", &c.s.CPUPercent, &lastCPU)
		locfI("cpu_cores", &c.s.CPUCores, &lastCPUCores)
		locfF("mem_percent", &c.s.MemPercent, &lastMem)
		locfU("mem_used_bytes", &c.s.MemUsed, &lastMemUsed)
		locfU("mem_total_bytes", &c.s.MemTotal, &lastMemTotal)
		locfF("swap_percent", &c.s.SwapPercent, &lastSwap)
		locfU("swap_used_bytes", &c.s.SwapUsed, &lastSwapUsed)
		locfU("swap_total_bytes", &c.s.SwapTotal, &lastSwapTotal)
		locfF("disk_percent", &c.s.DiskPercent, &lastDisk)
		locfU("disk_used_bytes", &c.s.DiskUsed, &lastDiskUsed)
		locfU("disk_total_bytes", &c.s.DiskTotal, &lastDiskTotal)
		locfU("uptime_seconds", &c.s.Uptime, &lastUptime)
		locfF("disk_io_util_percent", &c.s.DiskIOUtilPercent, &lastDiskIO)
		locfF("disk_read_rate", &c.s.DiskReadRate, &lastDiskRR)
		locfF("disk_write_rate", &c.s.DiskWriteRate, &lastDiskWR)
		locfF("disk_read_iops", &c.s.DiskReadIOPS, &lastDiskRI)
		locfF("disk_write_iops", &c.s.DiskWriteIOPS, &lastDiskWI)
		locfF("net_sent_rate", &c.s.NetSentRate, &lastNetS)
		locfF("net_recv_rate", &c.s.NetRecvRate, &lastNetR)
		locfI("net_conns", &c.s.NetConns, &lastNetConns)
		locfF("load1", &c.s.Load1, &lastLoad1)
		locfF("load5", &c.s.Load5, &lastLoad5)
		locfF("load15", &c.s.Load15, &lastLoad15)
		locfI("proc_count", &c.s.ProcCount, &lastProc)

		if len(c.gpuFields) > 0 {
			c.s.GPUs = mergeGPUHoldForward(lastGPUs, c.s.GPUs, c.gpuFields)
			lastGPUs = cloneGPUInfos(c.s.GPUs)
		} else if len(lastGPUs) > 0 {
			c.s.GPUs = cloneGPUInfos(lastGPUs)
		}
		if len(c.diskFields) > 0 {
			c.s.Disks = mergeDiskHoldForward(lastDisks, c.s.Disks, c.diskFields)
			lastDisks = cloneDiskInfos(c.s.Disks)
		} else if len(lastDisks) > 0 {
			c.s.Disks = cloneDiskInfos(lastDisks)
		}
		if c.connSet {
			c.s.Conns = mergeConnHoldForward(lastConns, c.s.Conns)
			lastConns = cloneConnInfos(c.s.Conns)
		} else if len(lastConns) > 0 {
			c.s.Conns = cloneConnInfos(lastConns)
		}

		stabilizeSampleArrays(&c.s)
		out = append(out, c.s)
	}
	return out
}

// alignSamplesToStep re-samples LOCF-joined history onto [from,to] at a fixed
// step. VictoriaMetrics query_range may omit empty evaluation slots on short
// windows (1h/3h), producing irregular density that makes multi-gauge charts
// flicker between N visible curves.
func alignSamplesToStep(samples []shared.Sample, from, to, step int64) []shared.Sample {
	if len(samples) == 0 || step < 1 {
		return samples
	}
	if from > to {
		from, to = to, from
	}
	// Align from upward to step boundary used by adaptiveHistoryStep callers.
	start := from
	if rem := start % step; rem != 0 {
		start += step - rem
	}
	if start > to {
		return samples
	}
	idx := 0
	var last *shared.Sample
	out := make([]shared.Sample, 0, int((to-start)/step)+2)
	for t := start; t <= to; t += step {
		for idx < len(samples) && samples[idx].Timestamp <= t {
			s := samples[idx]
			last = &s
			idx++
		}
		if last == nil {
			continue
		}
		row := *last
		row.Timestamp = t
		out = append(out, row)
	}
	if len(out) == 0 {
		return samples
	}
	return out
}

func cloneGPUInfos(in []shared.GPUInfo) []shared.GPUInfo {
	if len(in) == 0 {
		return nil
	}
	out := make([]shared.GPUInfo, len(in))
	copy(out, in)
	return out
}

func cloneDiskInfos(in []shared.DiskInfo) []shared.DiskInfo {
	if len(in) == 0 {
		return nil
	}
	out := make([]shared.DiskInfo, len(in))
	copy(out, in)
	return out
}

func cloneConnInfos(in []shared.ConnStat) []shared.ConnStat {
	if len(in) == 0 {
		return nil
	}
	out := make([]shared.ConnStat, len(in))
	copy(out, in)
	return out
}

func mergeGPUHoldForward(prev, cur []shared.GPUInfo, fields map[string]map[string]bool) []shared.GPUInfo {
	if len(prev) == 0 {
		return cur
	}
	if len(cur) == 0 {
		return cloneGPUInfos(prev)
	}
	byPrev := make(map[string]shared.GPUInfo, len(prev))
	for _, g := range prev {
		byPrev[g.Name] = g
	}
	by := make(map[string]shared.GPUInfo, len(prev)+len(cur))
	for _, g := range prev {
		by[g.Name] = g
	}
	for _, g := range cur {
		p, had := byPrev[g.Name]
		if !had {
			by[g.Name] = g
			continue
		}
		fset := fields[g.Name]
		merged := p
		merged.Name = g.Name
		if fset["util"] {
			merged.UtilPercent = g.UtilPercent
		}
		if fset["temp"] {
			merged.Temp = g.Temp
		}
		if fset["mem_pct"] {
			merged.MemPercent = g.MemPercent
		}
		if fset["mem_used"] {
			merged.MemUsed = g.MemUsed
		}
		if fset["mem_free"] {
			merged.MemFree = g.MemFree
		}
		if fset["mem_total"] {
			merged.MemTotal = g.MemTotal
		}
		by[g.Name] = merged
	}
	out := make([]shared.GPUInfo, 0, len(by))
	for _, g := range by {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func mergeDiskHoldForward(prev, cur []shared.DiskInfo, fields map[string]map[string]bool) []shared.DiskInfo {
	if len(prev) == 0 {
		return cur
	}
	if len(cur) == 0 {
		return cloneDiskInfos(prev)
	}
	byPrev := make(map[string]shared.DiskInfo, len(prev))
	for _, d := range prev {
		byPrev[d.Path] = d
	}
	by := make(map[string]shared.DiskInfo, len(prev)+len(cur))
	for _, d := range prev {
		by[d.Path] = d
	}
	for _, d := range cur {
		p, had := byPrev[d.Path]
		if !had {
			by[d.Path] = d
			continue
		}
		fset := fields[d.Path]
		merged := p
		merged.Path = d.Path
		if fset["percent"] {
			merged.Percent = d.Percent
		}
		if fset["used"] {
			merged.Used = d.Used
		}
		if fset["total"] {
			merged.Total = d.Total
		}
		by[d.Path] = merged
	}
	out := make([]shared.DiskInfo, 0, len(by))
	for _, d := range by {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func mergeConnHoldForward(prev, cur []shared.ConnStat) []shared.ConnStat {
	if len(prev) == 0 {
		return cur
	}
	if len(cur) == 0 {
		return cloneConnInfos(prev)
	}
	type key struct{ p, s string }
	by := make(map[key]shared.ConnStat, len(prev)+len(cur))
	for _, c := range prev {
		by[key{c.Proto, c.State}] = c
	}
	for _, c := range cur {
		by[key{c.Proto, c.State}] = c
	}
	out := make([]shared.ConnStat, 0, len(by))
	for _, c := range by {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Proto != out[j].Proto {
			return out[i].Proto < out[j].Proto
		}
		return out[i].State < out[j].State
	})
	return out
}
