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
