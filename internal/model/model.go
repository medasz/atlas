package model

import "time"

// Host 主机资产
type Host struct {
	IP        string         `json:"ip"`
	ASN       int            `json:"asn"`
	Org       string         `json:"org"`
	Geo       map[string]any `json:"geo"`
	OS        string         `json:"os"`
	IsIPv6    bool           `json:"is_ipv6"`
	OpenPorts []int          `json:"open_ports"`
	FirstSeen time.Time      `json:"first_seen"`
	LastSeen  time.Time      `json:"last_seen"`
}

// Port 端口/服务资产
type Port struct {
	IP      string         `json:"ip"`
	Port    int            `json:"port"`
	Proto   string         `json:"proto"`
	State   string         `json:"state"` // 端口状态：open|closed|filtered|timeout|open|filtered|unfiltered（来自多模式扫描）
	Service string         `json:"service"`
	Version string         `json:"version"`
	Banner  string         `json:"banner"`
	Title   string         `json:"title"`
	Host    string         `json:"host"`    // 到达该端口所用的主机名/域名（HTTP Host）
	IsIPv6  bool           `json:"is_ipv6"`
	Cert    map[string]any `json:"cert"`
	WebInfo map[string]any `json:"webinfo"`
	FirstSeen time.Time     `json:"first_seen"`
	LastSeen  time.Time     `json:"last_seen"`
}

// Domain 域名资产（含根域、解析、WHOIS 占位）
type Domain struct {
	Name              string    `json:"name"`               // 完整主机名
	RegistrableDomain string    `json:"registrable_domain"` // 注册根域
	ResolvedIPs       []string  `json:"resolved_ips"`
	CNAME             []string  `json:"cname"`
	Org               string    `json:"org"`
	ASN               int       `json:"asn"`
	IsIPv6            bool      `json:"is_ipv6"`
	Whois             map[string]any `json:"whois"`
	FirstSeen         time.Time `json:"first_seen"`
	LastSeen          time.Time `json:"last_seen"`
}

// BlacklistItem 黑名单条目（不扫描的资产）
type BlacklistItem struct {
	Type      string    `json:"type"` // ip | cidr | domain
	Value     string    `json:"value"`
	Operator  string    `json:"operator"`
	CreatedAt time.Time `json:"created_at"`
}

// Task 扫描/漏洞任务
type Task struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"` // scan | vuln
	Scope     map[string]any `json:"scope"`
	Schedule  map[string]any `json:"schedule"`
	RateLimit map[string]any `json:"rate_limit"`
	Status    int            `json:"status"` // 0 pending 1 running 2 done
	Progress  map[string]int `json:"progress"`
	CreatedAt time.Time      `json:"created_at"`
}

// Vuln 漏洞结果
type Vuln struct {
	AssetRef     string    `json:"asset_ref"`
	KPID         string    `json:"kpid"`
	CVE          string    `json:"cve"`
	Name         string    `json:"name"`
	Level        int       `json:"level"`
	Type         string    `json:"type"`
	Proof        string    `json:"proof"`
	Status       string    `json:"status"` // open|fixed|recur
	FirstFound   time.Time `json:"first_found"`
	LastVerified time.Time `json:"last_verified"`
}

// TaskItem 任务子项（断点续扫单元，粒度可细化到端口块）
type TaskItem struct {
	TaskID string         `json:"task_id"`
	Target string         `json:"target"`
	Ports  string         `json:"ports"`   // 端口块规格，如 "1-1000"；域名/空块为 ""
	Status int            `json:"status"` // 0 pending 1 done 2 filtered
	Result map[string]any `json:"result"`
}

// Task status 常量
const (
	TaskPending  = 0
	TaskRunning  = 1
	TaskDone     = 2
	TaskPaused   = 3
	TaskItemPending  = 0
	TaskItemDone     = 1
	TaskItemFiltered = 2
)

// Task 类型
const (
	TaskScan = "scan"
	TaskVuln = "vuln"
)
