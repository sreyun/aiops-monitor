package main

import (
	"testing"

	"aiops-monitor/shared"
)

// TestEmbeddedConfigExampleDecodes guards the shipped example config: it must
// always decode cleanly into the agent config struct through the real load path
// (shared.DecodeConfig with a .yaml extension). Without this, a typo or an
// indentation slip in config.example.yaml would only surface on a customer's
// first agent start — exactly where it hurts most.
func TestEmbeddedConfigExampleDecodes(t *testing.T) {
	var cfg config
	if err := shared.DecodeConfig("config.yaml", configExampleYAML, &cfg); err != nil {
		t.Fatalf("embedded config.example.yaml failed to decode: %v", err)
	}
	// Spot-check representative fields across the struct survived YAML → struct.
	if cfg.Server == "" {
		t.Error("server field did not decode")
	}
	if cfg.ReportInterval != 30 {
		t.Errorf("report_interval = %d, want 30", cfg.ReportInterval)
	}
	if cfg.PluginInterval != 60 {
		t.Errorf("plugin_interval = %d, want 60", cfg.PluginInterval)
	}
	if cfg.HyperVIntervalSec != 60 {
		t.Errorf("hyperv_interval_sec = %d, want 60", cfg.HyperVIntervalSec)
	}
	if !cfg.LogEncrypt {
		t.Error("log_encrypt should decode to true")
	}
}
