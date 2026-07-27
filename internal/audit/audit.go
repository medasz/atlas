package audit

import (
	"context"

	"atlas/internal/store"
)

// Auditor 审计记录器（受开关控制）
type Auditor struct {
	store   *store.Store
	enabled bool
}

// New 创建审计器
func New(s *store.Store, enabled bool) *Auditor {
	return &Auditor{store: s, enabled: enabled}
}

// Log 记录一条审计。开关关闭时直接返回 nil（不写任何记录）
func (a *Auditor) Log(ctx context.Context, operator, target, taskID, action string) error {
	if !a.enabled {
		return nil
	}
	return a.store.InsertAudit(ctx, operator, target, taskID, action)
}

// Enabled 返回审计是否开启
func (a *Auditor) Enabled() bool { return a.enabled }

// SetEnabled 热更新审计开关
func (a *Auditor) SetEnabled(v bool) { a.enabled = v }
