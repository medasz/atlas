package scope

import (
	"crypto/rand"
	"math/big"
)

// cyclicGroupEntry 存储指定位数/范围上限的素数 p 与原根 g
type cyclicGroupEntry struct {
	maxVal    uint64
	prime     uint64
	generator uint64
}

// precomputedGroups 预置常用的比特位/范围大小对应的素数 (p > maxVal) 与原根 (g)
var precomputedGroups = []cyclicGroupEntry{
	{maxVal: 2, prime: 3, generator: 2},
	{maxVal: 4, prime: 5, generator: 2},
	{maxVal: 8, prime: 11, generator: 2},
	{maxVal: 16, prime: 17, generator: 3},
	{maxVal: 32, prime: 37, generator: 2},
	{maxVal: 64, prime: 67, generator: 2},
	{maxVal: 128, prime: 131, generator: 2},
	{maxVal: 256, prime: 257, generator: 3}, // /24 CIDR (254/256)
	{maxVal: 512, prime: 521, generator: 3},
	{maxVal: 1024, prime: 1031, generator: 14},
	{maxVal: 2048, prime: 2053, generator: 3},
	{maxVal: 4096, prime: 4099, generator: 3},
	{maxVal: 8192, prime: 8209, generator: 7},
	{maxVal: 16384, prime: 16411, generator: 2},
	{maxVal: 32768, prime: 32771, generator: 2},
	{maxVal: 65536, prime: 65537, generator: 3}, // /16 CIDR (65534/65536)
	{maxVal: 131072, prime: 131101, generator: 2},
	{maxVal: 262144, prime: 262147, generator: 2},
	{maxVal: 524288, prime: 524309, generator: 2},
	{maxVal: 1048576, prime: 1048583, generator: 3},
	{maxVal: 2097152, prime: 2097169, generator: 6},
	{maxVal: 4194304, prime: 4194319, generator: 13},
	{maxVal: 8388608, prime: 8388617, generator: 5},
	{maxVal: 16777216, prime: 16777259, generator: 2}, // /8 CIDR
	{maxVal: 33554432, prime: 33554467, generator: 2},
	{maxVal: 67108864, prime: 67108879, generator: 3},
	{maxVal: 134217728, prime: 134217757, generator: 2},
	{maxVal: 268435456, prime: 268435459, generator: 2},
	{maxVal: 536870912, prime: 536870923, generator: 2},
	{maxVal: 1073741824, prime: 1073741827, generator: 2},
	{maxVal: 2147483648, prime: 2147483659, generator: 2},
	{maxVal: 4294967296, prime: 4294967311, generator: 3}, // /0 全网 IPv4 4294967296
}

// CyclicGroup 循环群伪随机序列生成器
type CyclicGroup struct {
	numValues   uint64 // 目标总数 N
	prime       uint64 // 素数 p
	generator   uint64 // 原根 g
	current     uint64 // 当前状态 x_k
	start       uint64 // 初始状态 x_0
	validCount  uint64 // 全局有效元素生成计数
	shardIndex  uint64 // 当前分片索引 (0-based)
	numShards   uint64 // 分片总数
	finished    bool   // 是否完成遍历
}

// NewCyclicGroup 创建一个新的循环群伪随机生成器。
// numValues: 目标元素个数 N (必须 >= 1)
// seed: 随机种子 (若为 0 则自动采用密码学随机数生成)
// shardIndex & numShards: 用于分布式分片，1 个节点时传 0 与 1。
func NewCyclicGroup(numValues uint64, seed uint64, shardIndex, numShards uint64) *CyclicGroup {
	if numValues == 0 {
		return &CyclicGroup{finished: true}
	}
	if numShards <= 0 {
		numShards = 1
	}
	if shardIndex >= numShards {
		shardIndex = 0
	}
	if seed == 0 {
		seed = secureRandomUint64()
	}

	prime, generator := findGroup(numValues)

	// 初始状态 x_0：结合 seed 计算起点
	startSeed := seed % (prime - 1)
	if startSeed == 0 {
		startSeed = 1
	}
	start := powMod(generator, startSeed, prime)

	cg := &CyclicGroup{
		numValues:  numValues,
		prime:      prime,
		generator:  generator,
		current:    start,
		start:      start,
		validCount: 0,
		shardIndex: shardIndex,
		numShards:  numShards,
		finished:   false,
	}

	return cg
}

// Next 返回下一个伪随机索引 (0 <= index < numValues)，如果已经遍历完毕则返回 (0, false)。
func (cg *CyclicGroup) Next() (uint64, bool) {
	if cg.finished || cg.numValues == 0 {
		return 0, false
	}

	// 特殊情况：当 N = 1 时
	if cg.numValues == 1 {
		if cg.validCount == 0 && cg.shardIndex == 0 {
			cg.validCount++
			cg.finished = true
			return 0, true
		}
		cg.finished = true
		return 0, false
	}

	for {
		curr := cg.current

		// 递推计算下一个状态: x_{k+1} = (x_k * g) % p
		cg.current = mulMod(cg.current, cg.generator, cg.prime)

		// 检查终止条件：如果重新绕回起点，说明一个周期已结束
		if cg.current == cg.start {
			cg.finished = true
		}

		// 拒绝采样：若产生的数字落在有效区间 1 <= curr <= numValues 内，则为有效目标
		if curr >= 1 && curr <= cg.numValues {
			itemIndex := cg.validCount
			cg.validCount++

			// 分片过滤：检查该有效目标属于哪个分片
			if itemIndex%cg.numShards == cg.shardIndex {
				return curr - 1, true
			}
		}

		// 如果重新绕回起点，跳出
		if cg.finished {
			return 0, false
		}
	}
}

// Reset 将生成器重置回初始状态
func (cg *CyclicGroup) Reset() {
	cg.current = cg.start
	cg.validCount = 0
	cg.finished = (cg.numValues == 0)
}

// Total 返回目标总数 N
func (cg *CyclicGroup) Total() uint64 {
	return cg.numValues
}

// findGroup 查找适合 numValues 的素数 p 与原根 g
func findGroup(numValues uint64) (uint64, uint64) {
	for _, entry := range precomputedGroups {
		if entry.maxVal >= numValues {
			return entry.prime, entry.generator
		}
	}
	// 默认兜底使用 2^32 的参数
	return 4294967311, 3
}

// mulMod 64 位无符号整数乘法模计算：(a * b) % m，防止 64 位溢出
func mulMod(a, b, m uint64) uint64 {
	ba := new(big.Int).SetUint64(a)
	bb := new(big.Int).SetUint64(b)
	bm := new(big.Int).SetUint64(m)
	res := new(big.Int).Mul(ba, bb)
	res.Mod(res, bm)
	return res.Uint64()
}

// powMod 64 位模幂计算：(base^exp) % m
func powMod(base, exp, m uint64) uint64 {
	bBase := new(big.Int).SetUint64(base)
	bExp := new(big.Int).SetUint64(exp)
	bM := new(big.Int).SetUint64(m)
	res := new(big.Int).Exp(bBase, bExp, bM)
	return res.Uint64()
}

func secureRandomUint64() uint64 {
	var buf [8]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		return 123456789
	}
	val := uint64(buf[0]) | uint64(buf[1])<<8 | uint64(buf[2])<<16 | uint64(buf[3])<<24 |
		uint64(buf[4])<<32 | uint64(buf[5])<<40 | uint64(buf[6])<<48 | uint64(buf[7])<<56
	if val == 0 {
		return 1
	}
	return val
}
