package main

import (
	"strings"
	"testing"
)

// The VM read path must reassemble per-series export lines into per-timestamp
// samples (join by ts, ms→s, correct field mapping, sorted).
func TestVMExportParse(t *testing.T) {
	nd := `{"metric":{"__name__":"aiops_cpu_percent","host":"h1"},"values":[50,60],"timestamps":[105000,100000]}
{"metric":{"__name__":"aiops_mem_percent","host":"h1"},"values":[70,80],"timestamps":[105000,100000]}
{"metric":{"__name__":"aiops_load1","host":"h1"},"values":[1.5],"timestamps":[100000]}`
	s := parseVMExport(strings.NewReader(nd))
	if len(s) != 2 {
		t.Fatalf("expected 2 samples (2 distinct timestamps), got %d", len(s))
	}
	// sorted ascending: ts=100 first
	if s[0].Timestamp != 100 || s[0].CPUPercent != 60 || s[0].MemPercent != 80 || s[0].Load1 != 1.5 {
		t.Errorf("sample@100 wrong: %+v", s[0])
	}
	if s[1].Timestamp != 105 || s[1].CPUPercent != 50 || s[1].MemPercent != 70 {
		t.Errorf("sample@105 wrong: %+v", s[1])
	}
}

// GPU 利用率在 VM 里是带 gpu 标签的独立系列（每块显卡一条），parseVMExport 必须按名
// 重建每个时间点的 GPUs 数组——否则历史读回缺 gpus，前端画不出「GPU 近期趋势图」（曾漏）。
func TestVMExportParseGPU(t *testing.T) {
	nd := `{"metric":{"__name__":"aiops_gpu_util_percent","host":"h1","gpu":"GPU0"},"values":[30,40],"timestamps":[100000,105000]}
{"metric":{"__name__":"aiops_gpu_util_percent","host":"h1","gpu":"GPU1"},"values":[55],"timestamps":[100000]}
{"metric":{"__name__":"aiops_cpu_percent","host":"h1"},"values":[10],"timestamps":[100000]}`
	s := parseVMExport(strings.NewReader(nd))
	if len(s) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(s))
	}
	if len(s[0].GPUs) != 2 { // ts=100：两块显卡都应重建出来
		t.Fatalf("sample@100 应重建 2 块 GPU，实际 %d：%+v", len(s[0].GPUs), s[0].GPUs)
	}
	byName := map[string]float64{}
	for _, g := range s[0].GPUs {
		byName[g.Name] = g.UtilPercent
	}
	if byName["GPU0"] != 30 || byName["GPU1"] != 55 {
		t.Errorf("sample@100 GPU 值错误：%+v", s[0].GPUs)
	}
	if len(s[1].GPUs) != 1 || s[1].GPUs[0].Name != "GPU0" || s[1].GPUs[0].UtilPercent != 40 { // ts=105：仅 GPU0
		t.Errorf("sample@105 GPU 重建错误：%+v", s[1].GPUs)
	}
}

func TestPasswordPolicy(t *testing.T) {
	good := []string{"Abcd123!", "P@ssw0rd", "aB3$aB3$", "Zx9#mnop", "长密码Ab1!x"}
	for _, p := range good {
		if !validatePasswordStrength(p) {
			t.Errorf("should accept strong password %q", p)
		}
	}
	bad := map[string]string{
		"":           "empty",
		"Ab1!xy":     "too short (6)",
		"abcdefg1!":  "no uppercase",
		"ABCDEFG1!":  "no lowercase",
		"Abcdefgh!":  "no digit",
		"Abcdefg12":  "no special",
		"abcdefgh":   "only lowercase",
	}
	for p, why := range bad {
		if validatePasswordStrength(p) {
			t.Errorf("should reject %q (%s)", p, why)
		}
	}
}

func TestRecordingPersistence(t *testing.T) {
	dir := t.TempDir()
	m := newTermManager()
	m.recDir = dir
	arch := termArchive{
		info:      termSessionInfo{ID: "sess1", Hostname: "web-01", Operator: "alice", Frames: 2},
		recording: []termRecordFrame{{Ts: 1, Type: "output", Data: "aGk="}, {Ts: 2, Type: "input", Data: "eA=="}},
	}
	m.persistRecording(arch)

	// Read the frames straight back from the file.
	if got := m.readRecordingFile("sess1"); len(got) != 2 {
		t.Fatalf("readRecordingFile: expected 2 frames, got %d", len(got))
	}

	// A fresh manager (simulating a restart) indexes the persisted recording...
	m2 := newTermManager()
	m2.loadRecordings(dir)
	var found *termSessionInfo
	for _, s := range m2.listSessions() {
		if s.ID == "sess1" {
			cp := s
			found = &cp
		}
	}
	if found == nil {
		t.Fatal("loadRecordings should index the persisted session after restart")
	}
	if found.Frames != 2 || found.Active {
		t.Errorf("restored session info wrong: frames=%d active=%v", found.Frames, found.Active)
	}
	// ...and replay reads the frames lazily from the file.
	if got := m2.getRecording("sess1"); len(got) != 2 {
		t.Fatalf("getRecording after restart should read 2 frames from file, got %d", len(got))
	}
	// Unknown session → nil (no panic).
	if m2.getRecording("nope") != nil {
		t.Error("unknown session should return nil")
	}
}
