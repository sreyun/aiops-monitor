package main

import "testing"

func TestThresholdPresetsIncludeSNMPAndNetFlow(t *testing.T) {
	for name, fn := range map[string]func() Thresholds{
		"conservative": ConservativeThresholds,
		"standard":     StandardThresholds,
		"relaxed":      RelaxedThresholds,
	} {
		th := fn()
		cfg := thresholdConfigFromThresholds(th)
		if cfg.SNMPIfUtilWarn <= 0 || cfg.SNMPIfUtilCrit <= 0 {
			t.Fatalf("%s missing SNMP util thresholds: %+v", name, cfg)
		}
		if cfg.NetFlowSurgeRatio <= 0 || cfg.NetFlowDropWarn <= 0 {
			t.Fatalf("%s missing NetFlow thresholds: %+v", name, cfg)
		}
	}
	std := thresholdConfigFromThresholds(StandardThresholds())
	if std != defaultThresholdConfig() {
		t.Fatalf("defaultThresholdConfig must match StandardThresholds\n got: %+v\nwant: %+v", defaultThresholdConfig(), std)
	}
}
