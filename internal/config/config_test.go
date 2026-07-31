package config

import (
	"testing"
)

func TestConfigKVMapping(t *testing.T) {
	cfg := defaultConfig()
	kv := configToKV(cfg)

	if kv["scan_default_mode"] != "connect" {
		t.Errorf("expected scan_default_mode=connect, got %s", kv["scan_default_mode"])
	}
	if kv["scan_max_concurrency"] != "500" {
		t.Errorf("expected scan_max_concurrency=500, got %s", kv["scan_max_concurrency"])
	}
	if kv["auth_enabled"] != "true" {
		t.Errorf("expected auth_enabled=true, got %s", kv["auth_enabled"])
	}

	// 验证反解析与修改覆盖
	newCfg := defaultConfig()
	kv["scan_default_mode"] = "syn"
	kv["scan_max_concurrency"] = "1000"
	kv["scan_record_closed_ports"] = "true"
	applyKVToConfig(newCfg, kv)

	if newCfg.Scan.DefaultMode != "syn" {
		t.Errorf("expected syn, got %s", newCfg.Scan.DefaultMode)
	}
	if newCfg.Scan.MaxConcurrency != 1000 {
		t.Errorf("expected 1000, got %d", newCfg.Scan.MaxConcurrency)
	}
	if newCfg.Scan.RecordClosedPorts != true {
		t.Errorf("expected RecordClosedPorts=true, got %v", newCfg.Scan.RecordClosedPorts)
	}
}
