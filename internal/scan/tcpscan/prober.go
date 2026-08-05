package tcpscan

import (
	"context"
	"net"
	"time"
)

// Options 单次扫描的可调参数。
// 调用方负责并行多个目标 IP（受 Concurrency 约束）；单 IP 内由实现决定发包策略。
type Options struct {
	// TaskID and PortRange are trace metadata supplied by the task engine. They
	// make raw packet path diagnostics attributable to one concrete task item.
	TaskID    string
	PortRange string
	Timeout   time.Duration // raw 抓包窗口基准 / connect 建连超时
	Retries   int           // 无响应重发次数（0 表示仅发 1 次）
	// Concurrency 由 atlas 外层任务调度（taskSvc）负责并行目标 IP，本包内不消费该字段。
	Concurrency int
	Iface       string // 抓包网卡（raw，空=自动选出口）
	SrcPort     int    // 源端口（0=随机高端口）
	SourceIP    net.IP // 发包源 IP（空=出口 IP）
	// OnRawFallback raw 句柄打不开时回调，便于调用方降级为 connect 并告警（非 OS 级降级）。
	OnRawFallback func(reason error)
	// InstallRstDrop 是否尝试安装 RST-drop 规则（stealth），默认应由配置开启。
	InstallRstDrop bool
}

// Scanner 统一扫描接口：按模式切换实现（connect / raw）。
type Scanner interface {
	// Mode 返回本实例对应的扫描模式。
	Mode() Mode
	// Scan 对单一目标 IP 的端口集合执行扫描，返回每端口结果。
	// 返回 map 始终包含请求的全部端口（含 closed/timeout 等负结果），
	// 因此调用方可据 PortState 持久化策略决定是否落库。
	Scan(ctx context.Context, target string, ports []int, opts Options) (map[int]Result, error)
}
