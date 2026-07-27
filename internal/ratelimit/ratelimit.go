package ratelimit

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// Limiter 令牌桶限速：全局桶 + 每目标桶
type Limiter struct {
	global       *rate.Limiter
	mu           sync.Mutex
	perTarget    map[string]*rate.Limiter
	perTargetRPS int
}

// New 创建限速器。globalRPS 为单实例全局最大并发/速率，perTargetRPS 为每目标速率
func New(globalRPS, perTargetRPS int) *Limiter {
	if globalRPS <= 0 {
		globalRPS = 1
	}
	if perTargetRPS <= 0 {
		perTargetRPS = 1
	}
	return &Limiter{
		global:       rate.NewLimiter(rate.Limit(globalRPS), globalRPS),
		perTarget:    make(map[string]*rate.Limiter),
		perTargetRPS: perTargetRPS,
	}
}

// WaitGlobal 等待全局令牌
func (l *Limiter) WaitGlobal(ctx context.Context) error {
	return l.global.Wait(ctx)
}

// WaitTarget 等待指定目标的令牌（按目标独立限流）
func (l *Limiter) WaitTarget(ctx context.Context, target string) error {
	l.mu.Lock()
	tl, ok := l.perTarget[target]
	if !ok {
		tl = rate.NewLimiter(rate.Limit(l.perTargetRPS), l.perTargetRPS)
		l.perTarget[target] = tl
	}
	l.mu.Unlock()
	return tl.Wait(ctx)
}

// SetLimits 热更新限速参数：重建全局令牌桶，并覆盖每目标速率（仅对新目标生效）
func (l *Limiter) SetLimits(globalRPS, perTargetRPS int) {
	if globalRPS <= 0 {
		globalRPS = 1
	}
	if perTargetRPS <= 0 {
		perTargetRPS = 1
	}
	l.mu.Lock()
	l.global = rate.NewLimiter(rate.Limit(globalRPS), globalRPS)
	l.perTargetRPS = perTargetRPS
	l.mu.Unlock()
}
