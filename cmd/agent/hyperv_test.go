package main

import (
	"reflect"
	"testing"
)

// TestParseHyperV covers the three JSON shapes PowerShell's ConvertTo-Json can
// emit — a normal array, a bare object (PS 5.1 drops brackets for a single VM),
// and the empty forms — plus field mapping and health normalization.
func TestParseHyperV(t *testing.T) {
	const multi = `[
	  {"Name":"web01","Id":"11111111-1111-1111-1111-111111111111","State":"Running","Status":"Operating normally","CPUUsage":12,"ProcessorCount":4,"MemAssignedMB":4096,"MemDemandMB":3000,"MemMaxMB":8192,"UptimeSec":3600,"Generation":2,"Version":"9.0","IP":"10.0.0.5,10.0.0.6","Switches":"vSwitch-Ext","VHDCount":1,"CheckpointCount":2,"ReplState":"Disabled","ReplHealth":"NotApplicable"},
	  {"Name":"db01","Id":"22222222-2222-2222-2222-222222222222","State":"Off","Status":"","CPUUsage":0,"ProcessorCount":2,"MemAssignedMB":0,"MemDemandMB":0,"MemMaxMB":4096,"UptimeSec":0,"Generation":1,"Version":"8.0","IP":"","Switches":"","VHDCount":2,"CheckpointCount":0,"ReplState":"Disabled","ReplHealth":"NotApplicable"}
	]`
	guests, err := parseHyperV(multi)
	if err != nil {
		t.Fatalf("parseHyperV(multi) err = %v", err)
	}
	if len(guests) != 2 {
		t.Fatalf("guests = %d, want 2", len(guests))
	}
	w := guests[0]
	if w.Name != "web01" || w.State != "Running" || w.Health != "OK" {
		t.Errorf("web01 = {%s,%s,%s}, want {web01,Running,OK}", w.Name, w.State, w.Health)
	}
	if w.CPUUsage != 12 || w.MemAssignedMB != 4096 || w.MemDemandMB != 3000 || w.CheckpointCount != 2 {
		t.Errorf("web01 numerics wrong: %+v", w)
	}
	if !reflect.DeepEqual(w.IPAddresses, []string{"10.0.0.5", "10.0.0.6"}) {
		t.Errorf("web01 IPs = %v, want [10.0.0.5 10.0.0.6]", w.IPAddresses)
	}
	if !reflect.DeepEqual(w.Switches, []string{"vSwitch-Ext"}) {
		t.Errorf("web01 switches = %v", w.Switches)
	}
	if d := guests[1]; d.State != "Off" || d.Health != "" || len(d.IPAddresses) != 0 {
		t.Errorf("db01 = {%s,%q,%v}, want {Off,\"\",[]}", d.State, d.Health, d.IPAddresses)
	}

	// Single VM: ConvertTo-Json emits a bare object; parseHyperV must wrap it.
	const single = `{"Name":"solo","Id":"x","State":"Paused","CPUUsage":0,"IP":"192.168.1.9"}`
	sg, err := parseHyperV(single)
	if err != nil {
		t.Fatalf("parseHyperV(single) err = %v", err)
	}
	if len(sg) != 1 || sg[0].Name != "solo" || sg[0].Health != "Warning" {
		t.Fatalf("single = %+v, want 1 guest solo/Warning", sg)
	}
	if !reflect.DeepEqual(sg[0].IPAddresses, []string{"192.168.1.9"}) {
		t.Errorf("solo IP = %v", sg[0].IPAddresses)
	}

	// Empty forms must all yield zero guests, no error.
	for _, in := range []string{"[]", "null", "", "   \n "} {
		g, err := parseHyperV(in)
		if err != nil || len(g) != 0 {
			t.Errorf("parseHyperV(%q) = (%v, %v), want (0 guests, nil)", in, g, err)
		}
	}

	// A guest with no name is dropped (can't key/label it).
	g, _ := parseHyperV(`[{"Name":"","State":"Running"},{"Name":"ok","State":"Running"}]`)
	if len(g) != 1 || g[0].Name != "ok" {
		t.Errorf("nameless guest not dropped: %+v", g)
	}
}

// TestParseHyperVDetail covers the expanded per-VM detail: nested Disks/Nics/
// Checkpoints (with the single-element-collapses-to-object quirk) and nic IPs
// arriving comma-joined.
func TestParseHyperVDetail(t *testing.T) {
	const in = `[{"Name":"web","Id":"g1","State":"Running",
	  "MemStartupMB":2048,"MemMinMB":512,"IntegrationState":"Up to date",
	  "Nics":{"Name":"NIC1","MAC":"00:15:5D:01:02:03","Switch":"vSwitch","Status":"Ok","Connected":true,"IP":"10.0.0.5,10.0.0.6"},
	  "Disks":[{"Path":"C:\\vm\\web.vhdx","ControllerType":"SCSI","ControllerNumber":0,"ControllerLocation":0,"FileSizeGB":42.5},
	           {"Path":"C:\\vm\\data.vhdx","ControllerType":"SCSI","FileSizeGB":100}],
	  "Checkpoints":{"Name":"before-patch","Created":"2026-07-18T01:02:03","Parent":""}}]`
	g, err := parseHyperV(in)
	if err != nil || len(g) != 1 {
		t.Fatalf("parse err=%v guests=%d", err, len(g))
	}
	v := g[0]
	if v.MemStartupMB != 2048 || v.MemMinMB != 512 || v.IntegrationState != "Up to date" {
		t.Errorf("mem/integration wrong: %+v", v)
	}
	if len(v.Nics) != 1 || v.Nics[0].MAC != "00:15:5D:01:02:03" || v.Nics[0].Switch != "vSwitch" || !v.Nics[0].Connected {
		t.Fatalf("nic wrong: %+v", v.Nics)
	}
	if len(v.Nics[0].IPAddresses) != 2 {
		t.Errorf("nic IPs = %v, want 2", v.Nics[0].IPAddresses)
	}
	if len(v.Disks) != 2 || v.Disks[0].ControllerType != "SCSI" || v.Disks[0].FileSizeGB != 42.5 || v.Disks[1].FileSizeGB != 100 {
		t.Fatalf("disks wrong: %+v", v.Disks)
	}
	if len(v.Checkpoints) != 1 || v.Checkpoints[0].Name != "before-patch" {
		t.Fatalf("checkpoints wrong: %+v", v.Checkpoints)
	}
}

func TestSplitComma(t *testing.T) {
	cases := map[string][]string{
		"":            nil,
		"   ":         nil,
		"a":           {"a"},
		"a,b,c":       {"a", "b", "c"},
		" a , , b ,":  {"a", "b"},
		"10.0.0.1,,,": {"10.0.0.1"},
	}
	for in, want := range cases {
		if got := splitComma(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitComma(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestHypervHealth(t *testing.T) {
	cases := []struct {
		state, repl, want string
	}{
		{"RunningCritical", "NotApplicable", "Critical"}, // 存储掉线等
		{"OffCritical", "", "Critical"},
		{"Running", "Critical", "Critical"}, // 复制严重
		{"Running", "Warning", "Warning"},
		{"Running", "Normal", "OK"},
		{"Paused", "NotApplicable", "Warning"},
		{"Saved", "", "Warning"},
		{"Off", "NotApplicable", ""}, // 关机不算"不健康"，交给告警层的状态规则
		{"Starting", "", ""},         // 过渡态
	}
	for _, c := range cases {
		if got := hypervHealth(c.state, c.repl); got != c.want {
			t.Errorf("hypervHealth(%q,%q) = %q, want %q", c.state, c.repl, got, c.want)
		}
	}
}
