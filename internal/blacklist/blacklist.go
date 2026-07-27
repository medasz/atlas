package blacklist

import (
	"context"
	"fmt"
	"net"
	"strings"

	"atlas/internal/audit"
	"atlas/internal/model"
	"atlas/internal/store"
)

// Service 黑名单管理：入库、命中判定、审计
type Service struct {
	store *store.Store
	audit *audit.Auditor
}

// New 构造黑名单服务
func New(s *store.Store, a *audit.Auditor) *Service {
	return &Service{store: s, audit: a}
}

// Add 新增黑名单条目并记录审计
func (svc *Service) Add(ctx context.Context, operator string, item model.BlacklistItem) error {
	if item.Type != "ip" && item.Type != "cidr" && item.Type != "domain" {
		return fmt.Errorf("invalid blacklist type: %q", item.Type)
	}
	if item.Value == "" {
		return fmt.Errorf("blacklist value required")
	}
	if item.Type == "cidr" {
		if _, _, err := net.ParseCIDR(item.Value); err != nil {
			return fmt.Errorf("invalid cidr %q: %w", item.Value, err)
		}
	}
	if err := svc.store.AddBlacklist(ctx, item); err != nil {
		return err
	}
	if svc.audit.Enabled() {
		_ = svc.audit.Log(ctx, operator, item.Value, "", "blacklist.add")
	}
	return nil
}

// List 列出全部黑名单条目
func (svc *Service) List(ctx context.Context) ([]model.BlacklistItem, error) {
	return svc.store.ListBlacklist(ctx)
}

// Remove 删除黑名单条目并记录审计
func (svc *Service) Remove(ctx context.Context, operator, typ, value string) error {
	if err := svc.store.DeleteBlacklist(ctx, typ, value); err != nil {
		return err
	}
	if svc.audit.Enabled() {
		_ = svc.audit.Log(ctx, operator, value, "", "blacklist.remove")
	}
	return nil
}

// Match 判定目标是否命中黑名单（支持 ip / cidr / domain）
func (svc *Service) Match(ctx context.Context, target string) (bool, error) {
	entries, err := svc.store.BlacklistEntries(ctx)
	if err != nil {
		return false, err
	}
	return matchEntries(target, entries), nil
}

// matchEntries 纯函数：给定黑名单条目集合与待判定目标，返回是否命中
func matchEntries(target string, entries []model.BlacklistItem) bool {
	ip := net.ParseIP(target)
	isIP := ip != nil
	for _, e := range entries {
		switch e.Type {
		case "ip":
			if e.Value == target {
				return true
			}
		case "cidr":
			if !isIP {
				continue
			}
			_, netC, err := net.ParseCIDR(e.Value)
			if err != nil {
				continue
			}
			if netC.Contains(ip) {
				return true
			}
		case "domain":
			if target == e.Value || strings.HasSuffix(target, "."+e.Value) {
				return true
			}
		}
	}
	return false
}
