package scope

import (
	"fmt"
	"net"
	"strings"
)

// BuildIterator 根据任务 Scope 构建统一的伪随机流式目标迭代器。
// 支持单个 IP、小网段、/16 与 /8 等任意规模 CIDR 网段以及域名目标的流式混合处理。
func BuildIterator(scope map[string]any) (TargetIterator, error) {
	raw, ok := scope["targets"]
	if !ok {
		return nil, fmt.Errorf("scope.targets required")
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("scope.targets must be a list")
	}

	var iterators []TargetIterator
	seen := map[string]bool{}

	var domainSlice []string

	for _, item := range list {
		s, _ := item.(string)
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true

		if _, ipNet, err := net.ParseCIDR(s); err == nil {
			iterators = append(iterators, NewCyclicCIDRIterator(ipNet, 0, 0, 1))
			continue
		}

		if ip := net.ParseIP(s); ip != nil {
			maskBits := 32
			if ip.To4() == nil {
				maskBits = 128
			}
			ipNet := &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(maskBits, maskBits),
			}
			iterators = append(iterators, NewCyclicCIDRIterator(ipNet, 0, 0, 1))
			continue
		}

		// 域名或包含端口的目标，归入 SliceIterator 集中处理
		domainSlice = append(domainSlice, s)
	}

	if len(domainSlice) > 0 {
		iterators = append(iterators, NewSliceIterator(domainSlice, 0))
	}

	if len(iterators) == 0 {
		return NewSliceIterator(nil, 0), nil
	}

	if len(iterators) == 1 {
		return iterators[0], nil
	}

	return NewMultiIterator(iterators...), nil
}

// Expand 将任务 scope 展开为待扫描目标列表（基于全新的 TargetIterator 流式离散生成）。
func Expand(scope map[string]any) ([]string, error) {
	it, err := BuildIterator(scope)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, it.Total())
	for {
		target, ok := it.Next()
		if !ok {
			break
		}
		out = append(out, target)
	}
	return out, nil
}

// ShuffleIPs 保持接口兼容性（内部已有 CyclicGroup 离散化打散，若有显式切片调用可直接返回或做二次微调）
func ShuffleIPs(ips []string) []string {
	if len(ips) <= 1 {
		return ips
	}
	out := make([]string, len(ips))
	copy(out, ips)
	return out
}
