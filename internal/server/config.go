package server

import "github.com/gin-gonic/gin"

// configPayload 前端提交的可变配置项
type configPayload struct {
	Scan struct {
		DefaultMode      string `json:"default_mode"`
		DefaultPortRange string `json:"default_port_range"`
		MaxConcurrency   int    `json:"max_concurrency"`
		PerTargetRPS     int    `json:"per_target_rps"`
		RawIface          string `json:"raw_iface"`
	} `json:"scan"`
	Audit struct {
		Enabled bool `json:"enabled"`
	} `json:"audit"`
}

// registerConfig 暴露速率与审计开关的配置读写接口
func (s *Server) registerConfig(g *gin.RouterGroup) {
	g.GET("/config", s.getConfig)
	g.PUT("/config", s.updateConfig)
}

func (s *Server) getConfig(c *gin.Context) {
	cfg := s.deps.Cfg
	c.JSON(200, gin.H{
		"scan": gin.H{
			"default_mode":       cfg.Scan.DefaultMode,
			"default_port_range": cfg.Scan.DefaultPortRange,
			"max_concurrency":    cfg.Scan.MaxConcurrency,
			"per_target_rps":     cfg.Scan.PerTargetRPS,
			"raw_iface":          cfg.Scan.RawIface,
		},
		"audit": gin.H{"enabled": s.deps.Audit.Enabled()},
	})
}

func (s *Server) updateConfig(c *gin.Context) {
	var p configPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	cfg := s.deps.Cfg
	if p.Scan.DefaultMode != "" {
		cfg.Scan.DefaultMode = p.Scan.DefaultMode
	}
	if p.Scan.DefaultPortRange != "" {
		cfg.Scan.DefaultPortRange = p.Scan.DefaultPortRange
	}
	if p.Scan.MaxConcurrency > 0 {
		cfg.Scan.MaxConcurrency = p.Scan.MaxConcurrency
	}
	if p.Scan.PerTargetRPS > 0 {
		cfg.Scan.PerTargetRPS = p.Scan.PerTargetRPS
	}
	// raw_iface 恒等更新（空字符串表示自动选出口网卡，可经界面重置）
	cfg.Scan.RawIface = p.Scan.RawIface

	// 运行时热更新：审计开关 + 限速器 + 扫描配置（模式/网卡，无需重启即对新建任务生效）
	s.deps.Audit.SetEnabled(p.Audit.Enabled)
	s.deps.Rate.SetLimits(cfg.Scan.MaxConcurrency, cfg.Scan.PerTargetRPS)
	if s.deps.Scanner != nil {
		s.deps.Scanner.SetScanConfig(cfg.Scan)
	}

	// 持久化到 YAML；若无法持久化仍保证内存生效
	if err := cfg.Save(); err != nil {
		c.JSON(200, gin.H{
			"warning": "配置已生效（运行时），但未持久化到文件: " + err.Error(),
			"audit":   gin.H{"enabled": s.deps.Audit.Enabled()},
			"scan": gin.H{
				"default_mode":       cfg.Scan.DefaultMode,
				"default_port_range": cfg.Scan.DefaultPortRange,
				"max_concurrency":    cfg.Scan.MaxConcurrency,
				"per_target_rps":     cfg.Scan.PerTargetRPS,
				"raw_iface":          cfg.Scan.RawIface,
			},
		})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
