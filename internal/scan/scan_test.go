package scan

import (
	"testing"

	"atlas/internal/config"
	"atlas/internal/ratelimit"
)

// TestSetScanConfigHotUpdate 验证运行时改模式/网卡后，扫描器能立即读到新配置，
// 即新建任务无需重启即可使用更新后的模式/网卡（修复前扫描器持有启动快照，热更新不可见）。
func TestSetScanConfigHotUpdate(t *testing.T) {
	lim := ratelimit.New(10, 5)
	sc := New(nil, lim, nil, nil, config.ScanConfig{DefaultMode: "connect"})
	if got := sc.liveScanCfg().DefaultMode; got != "connect" {
		t.Fatalf("初始模式应为 connect, 实际 %s", got)
	}
	sc.SetScanConfig(config.ScanConfig{DefaultMode: "syn", RawIface: "eth0"})
	if got := sc.liveScanCfg().DefaultMode; got != "syn" {
		t.Errorf("热更新后模式应为 syn, 实际 %s", got)
	}
	if got := sc.liveScanCfg().RawIface; got != "eth0" {
		t.Errorf("热更新后网卡应为 eth0, 实际 %s", got)
	}
	snapshot := sc.ScanConfigSnapshot()
	if snapshot.DefaultMode != "syn" || snapshot.RawIface != "eth0" {
		t.Errorf("snapshot = %+v, want mode=syn iface=eth0", snapshot)
	}
}
