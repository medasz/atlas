package task

import (
	"testing"

	"atlas/internal/scan"
)

func TestChunkSpecContiguous(t *testing.T) {
	ports := make([]int, 65535)
	for i := range ports {
		ports[i] = i + 1
	}
	chunks := chunkSpec(ports, 1000)
	if len(chunks) != 66 {
		t.Fatalf("expected 66 chunks, got %d", len(chunks))
	}
	if chunks[0] != "1-1000" {
		t.Fatalf("first chunk = %q, want 1-1000", chunks[0])
	}
	if chunks[65] != "65001-65535" {
		t.Fatalf("last chunk = %q, want 65001-65535", chunks[65])
	}
	// 还原校验：ParsePortSpec 必须精确重建原集合
	got, err := scan.ParsePortSpec(chunks[0])
	if err != nil || len(got) != 1000 || got[0] != 1 || got[999] != 1000 {
		t.Fatalf("round-trip failed: %v len=%d", err, len(got))
	}
}

func TestChunkSpecScattered(t *testing.T) {
	// 模拟 TopPorts 这种非连续集合
	ports := []int{21, 22, 23, 53, 80, 110, 443}
	chunks := chunkSpec(ports, 1000) // 单块（超过长度）
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	got, err := scan.ParsePortSpec(chunks[0])
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(got) != len(ports) {
		t.Fatalf("round-trip mismatch: got %d ports, want %d", len(got), len(ports))
	}
	for i, p := range got {
		if p != ports[i] {
			t.Fatalf("round-trip order mismatch at %d: %d != %d", i, p, ports[i])
		}
	}
}

func TestChunkSpecSizeOne(t *testing.T) {
	ports := []int{10, 20, 30}
	chunks := chunkSpec(ports, 1)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0] != "10-10" || chunks[2] != "30-30" {
		t.Fatalf("unexpected chunks: %v", chunks)
	}
}
