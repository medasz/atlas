package scope

import (
	"net"
	"testing"
)

// TestCyclicGroupCoverage 验证循环群算法对不同大小范围的覆盖率 (不重不漏)
func TestCyclicGroupCoverage(t *testing.T) {
	sizes := []uint64{1, 2, 5, 10, 254, 500, 1000, 65534}

	for _, n := range sizes {
		t.Run("Size_"+string(rune(n)), func(t *testing.T) {
			cg := NewCyclicGroup(n, 12345, 0, 1)
			visited := make(map[uint64]bool)

			count := uint64(0)
			for {
				val, ok := cg.Next()
				if !ok {
					break
				}
				count++
				if val >= n {
					t.Fatalf("N=%d: 生成值 %d 超过界限", n, val)
				}
				if visited[val] {
					t.Fatalf("N=%d: 检测到重复元素 %d", n, val)
				}
				visited[val] = true
			}

			if count != n {
				t.Fatalf("N=%d: 预期生成 %d 个元素，实际生成 %d 个", n, n, count)
			}
			if uint64(len(visited)) != n {
				t.Fatalf("N=%d: 访问去重集合大小 %d 与预期 %d 不符", n, len(visited), n)
			}
		})
	}
}

// TestCyclicGroupSharding 验证分布式 Sharding 分片的互斥性与完整性
func TestCyclicGroupSharding(t *testing.T) {
	totalN := uint64(1000)
	numShards := uint64(4)
	seed := uint64(9999)

	allVisited := make(map[uint64]int)

	for shard := uint64(0); shard < numShards; shard++ {
		cg := NewCyclicGroup(totalN, seed, shard, numShards)
		for {
			val, ok := cg.Next()
			if !ok {
				break
			}
			allVisited[val]++
		}
	}

	// 验证所有生成的元素不重复，且覆盖全量集合
	for i := uint64(0); i < totalN; i++ {
		c, ok := allVisited[i]
		if !ok {
			t.Fatalf("Sharding 漏掉了元素 %d", i)
		}
		if c > 1 {
			t.Fatalf("Sharding 重复扫描了元素 %d (%d 次)", i, c)
		}
	}
}

// TestCyclicCIDRIterator 测试 CIDR 伪随机迭代器 (以 /24 为例)
func TestCyclicCIDRIterator(t *testing.T) {
	_, ipNet, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR 失败: %v", err)
	}

	it := NewCyclicCIDRIterator(ipNet, 8888, 0, 1)

	// /24 网段去掉 .0 (网络) 和 .255 (广播)，应该有 254 个有效 IP
	if it.Total() != 254 {
		t.Fatalf("/24 有效 IP 预期 254，实际为 %d", it.Total())
	}

	seen := make(map[string]bool)
	count := 0
	for {
		ipStr, ok := it.Next()
		if !ok {
			break
		}
		count++
		if seen[ipStr] {
			t.Fatalf("CIDR 迭代生成了重复 IP: %s", ipStr)
		}
		seen[ipStr] = true

		// 确认不包含 .0 和 .255
		if ipStr == "192.168.1.0" || ipStr == "192.168.1.255" {
			t.Fatalf("不应包含网络号或广播地址: %s", ipStr)
		}
	}

	if count != 254 {
		t.Fatalf("预期生成 254 个 IP，实际生成 %d 个", count)
	}
}

// TestLargeCIDR 验证超大网段 (如 /16) 流式遍历
func TestLargeCIDR(t *testing.T) {
	_, ipNet, err := net.ParseCIDR("10.0.0.0/16")
	if err != nil {
		t.Fatalf("ParseCIDR 失败: %v", err)
	}

	it := NewCyclicCIDRIterator(ipNet, 123, 0, 1)

	if it.Total() != 65534 {
		t.Fatalf("/16 网段有效 IP 应为 65534，实际为 %d", it.Total())
	}

	count := 0
	for {
		_, ok := it.Next()
		if !ok {
			break
		}
		count++
	}

	if count != 65534 {
		t.Fatalf("/16 生成数量与预期不符: %d vs 65534", count)
	}
}

// TestMultiIterator 测试组合迭代器 (CIDR + Domain + IP)
func TestMultiIterator(t *testing.T) {
	_, ipNet, _ := net.ParseCIDR("172.16.0.0/29") // 6 个有效 IP
	cidrIt := NewCyclicCIDRIterator(ipNet, 1, 0, 1)

	domains := []string{"example.com", "test.org"}
	sliceIt := NewSliceIterator(domains, 2)

	multi := NewMultiIterator(cidrIt, sliceIt)

	if multi.Total() != 8 {
		t.Fatalf("MultiIterator 总数预期 8，实际为 %d", multi.Total())
	}

	count := 0
	for {
		_, ok := multi.Next()
		if !ok {
			break
		}
		count++
	}

	if count != 8 {
		t.Fatalf("MultiIterator 生成数量与预期不符: %d vs 8", count)
	}
}
