package scope

import (
	"encoding/binary"
	"net"
)

// TargetIterator 目标流式迭代器接口
type TargetIterator interface {
	Next() (target string, ok bool)
	Total() uint64
	Reset()
}

// CyclicCIDRIterator 基于 ZMap 循环群算法的 CIDR 伪随机目标迭代器
type CyclicCIDRIterator struct {
	ipNet     *net.IPNet
	baseIP    uint32
	validLen  uint64
	cg        *CyclicGroup
	skipNetBr bool // 是否为 IPv4 并剔除网络号与广播地址
}

// NewCyclicCIDRIterator 创建基于 CIDR 的循环群伪随机迭代器
func NewCyclicCIDRIterator(ipNet *net.IPNet, seed uint64, shardIndex, numShards uint64) *CyclicCIDRIterator {
	maskOnes, maskBits := ipNet.Mask.Size()
	hostBits := maskBits - maskOnes

	var totalHosts uint64 = 1 << uint(hostBits)
	ip4 := ipNet.IP.To4()

	var baseIP uint32
	var skipNetBr bool

	if ip4 != nil {
		baseIP = binary.BigEndian.Uint32(ip4)
		// 如果主机位 >= 2 (如 /30, /24, /16 等)，并且总主机数 >= 4，剔除网络地址与广播地址
		if hostBits >= 2 && totalHosts >= 4 {
			totalHosts -= 2
			skipNetBr = true
		}
	}

	cg := NewCyclicGroup(totalHosts, seed, shardIndex, numShards)

	return &CyclicCIDRIterator{
		ipNet:     ipNet,
		baseIP:    baseIP,
		validLen:  totalHosts,
		cg:        cg,
		skipNetBr: skipNetBr,
	}
}

// Next 获取下一个伪随机 IP
func (it *CyclicCIDRIterator) Next() (string, bool) {
	offset, ok := it.cg.Next()
	if !ok {
		return "", false
	}

	// 如果剔除了网络地址，实际偏移量加 1 跳过网络号 (如 .0)
	actualOffset := offset
	if it.skipNetBr {
		actualOffset += 1
	}

	targetUint := it.baseIP + uint32(actualOffset)
	ipBuf := make(net.IP, 4)
	binary.BigEndian.PutUint32(ipBuf, targetUint)

	return ipBuf.String(), true
}

// Total 返回当前 CIDR 有效 IP 数量
func (it *CyclicCIDRIterator) Total() uint64 {
	return it.validLen
}

// Reset 重置迭代器
func (it *CyclicCIDRIterator) Reset() {
	it.cg.Reset()
}

// SliceIterator 适用于显式列表（如域名或单 IP 列表）的迭代器
type SliceIterator struct {
	items   []string
	cg      *CyclicGroup
	total   uint64
	visited uint64
}

// NewSliceIterator 创建基于物理切片的伪随机迭代器
func NewSliceIterator(items []string, seed uint64) *SliceIterator {
	total := uint64(len(items))
	cg := NewCyclicGroup(total, seed, 0, 1)
	return &SliceIterator{
		items: items,
		cg:    cg,
		total: total,
	}
}

// Next 获取切片中的下一个伪随机元素
func (it *SliceIterator) Next() (string, bool) {
	idx, ok := it.cg.Next()
	if !ok || idx >= it.total {
		return "", false
	}
	it.visited++
	return it.items[idx], true
}

// Total 返回切片元素总数
func (it *SliceIterator) Total() uint64 {
	return it.total
}

// Reset 重置迭代器
func (it *SliceIterator) Reset() {
	it.cg.Reset()
	it.visited = 0
}

// MultiIterator 将多个迭代器组合为一个统一的迭代器
type MultiIterator struct {
	iterators []TargetIterator
	currIdx   int
	total     uint64
}

// NewMultiIterator 创建组合迭代器
func NewMultiIterator(its ...TargetIterator) *MultiIterator {
	var total uint64
	var valid []TargetIterator
	for _, it := range its {
		if it != nil && it.Total() > 0 {
			valid = append(valid, it)
			total += it.Total()
		}
	}
	return &MultiIterator{
		iterators: valid,
		currIdx:   0,
		total:     total,
	}
}

// Next 流式提取下一个目标
func (mi *MultiIterator) Next() (string, bool) {
	for mi.currIdx < len(mi.iterators) {
		val, ok := mi.iterators[mi.currIdx].Next()
		if ok {
			return val, true
		}
		mi.currIdx++
	}
	return "", false
}

// Total 返回所有迭代器的目标总数
func (mi *MultiIterator) Total() uint64 {
	return mi.total
}

// Reset 重置所有内部迭代器
func (mi *MultiIterator) Reset() {
	mi.currIdx = 0
	for _, it := range mi.iterators {
		it.Reset()
	}
}
