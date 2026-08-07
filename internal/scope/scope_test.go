package scope

import (
	"testing"
)

func TestShuffleIPs(t *testing.T) {
	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5"}
	shuffled := ShuffleIPs(ips)
	if len(shuffled) != len(ips) {
		t.Fatalf("expected len=%d, got %d", len(ips), len(shuffled))
	}

	m := map[string]bool{}
	for _, v := range shuffled {
		m[v] = true
	}
	for _, v := range ips {
		if !m[v] {
			t.Errorf("missing ip %s in shuffled output", v)
		}
	}
}

func TestExpandUnified(t *testing.T) {
	scopeMap := map[string]any{
		"targets": []any{
			"192.168.1.1",
			"10.0.0.0/30", // 2 个有效 IP (10.0.0.1, 10.0.0.2)
			"example.com",
		},
	}

	targets, err := Expand(scopeMap)
	if err != nil {
		t.Fatalf("Expand 失败: %v", err)
	}

	// 1 + 2 + 1 = 4
	if len(targets) != 4 {
		t.Fatalf("预期展开 4 个目标，实际得到 %d 个", len(targets))
	}

	seen := make(map[string]bool)
	for _, tName := range targets {
		seen[tName] = true
	}

	expected := []string{"192.168.1.1", "10.0.0.1", "10.0.0.2", "example.com"}
	for _, exp := range expected {
		if !seen[exp] {
			t.Errorf("缺失预期目标: %s", exp)
		}
	}
}
