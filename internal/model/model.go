package model

import "time"

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
	Status    int            `json:"status"` // 0 pending 1 running 2 done 3 paused 4 failed
	Progress  map[string]int `json:"progress"`
	Error     string         `json:"error,omitempty"` // 失败原因
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
	TaskID    string         `json:"task_id"`
	Target    string         `json:"target"`
	Ports     string         `json:"ports"`     // 端口块规格，如 "1-1000"；域名/空块为 ""
	Status    int            `json:"status"`    // 0 pending 1 processing 2 done 3 filtered 4 failed
	Result    map[string]any `json:"result"`
	Error     string         `json:"error,omitempty"`
	Attempts  int            `json:"attempts,omitempty"`
	LeaseUntil *time.Time    `json:"lease_until,omitempty"`
}

// Task status 常量
const (
	TaskPending     = 0
	TaskRunning     = 1
	TaskDone        = 2
	TaskPaused      = 3
	TaskFailed      = 4
	TaskItemPending    = 0
	TaskItemProcessing = 1
	TaskItemDone       = 2
	TaskItemFiltered   = 3
	TaskItemFailed     = 4
)

// Task 类型
const (
	TaskScan = "scan"
	TaskVuln = "vuln"
)
