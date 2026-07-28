package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"atlas/internal/audit"
	"atlas/internal/blacklist"
	"atlas/internal/config"
	"atlas/internal/fingerprint"
	"atlas/internal/queue"
	"atlas/internal/ratelimit"
	"atlas/internal/scan"
	"atlas/internal/store"
	"atlas/internal/task"
	"atlas/internal/vuln"

	"github.com/gin-gonic/gin"
)

// Deps 汇聚各层依赖，供路由处理器使用
type Deps struct {
	Cfg        *config.Config
	Store      *store.Store
	Queue      *queue.Queue
	Audit      *audit.Auditor
	Rate       *ratelimit.Limiter
	Blacklist  *blacklist.Service
	Task       *task.Service
	Scanner    *scan.Scanner // 资产探测引擎（配置热更新推送目标）
	Fingerprint *fingerprint.Service
	Vuln       *vuln.Engine
	WebDir     string // 非空时托管前端静态目录（单容器部署）
}

// Server HTTP 服务（gin）
type Server struct {
	eng  *gin.Engine
	deps Deps
}

// New 构造服务并注册路由
func New(d Deps) *Server {
	gin.SetMode(gin.ReleaseMode)
	eng := gin.New()
	eng.Use(gin.Recovery())
	eng.Use(cors())
	s := &Server{eng: eng, deps: d}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.eng.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	api := s.eng.Group("/api")
	api.Use(s.authRequired()) // 仅保护 /api，SPA 静态资源与根路径保持公开
	s.registerAuth(api)
	s.registerBlacklist(api)
	s.registerTasks(api)
	s.registerFingerprint(api)
	s.registerAssets(api)
	s.registerVuln(api)
	s.registerConfig(api)

	// 可选：托管前端 SPA（非 /api 的 GET 回退到 index.html）
	if s.deps.WebDir != "" {
		s.eng.NoRoute(func(c *gin.Context) {
			if c.Request.Method != http.MethodGet || strings.HasPrefix(c.Request.URL.Path, "/api") {
				c.Status(http.StatusNotFound)
				return
			}
			target := filepath.Join(s.deps.WebDir, c.Request.URL.Path)
			if info, err := os.Stat(target); err == nil && !info.IsDir() {
				c.File(target)
				return
			}
			c.File(filepath.Join(s.deps.WebDir, "index.html"))
		})
	}
}

// Run 启动 HTTP 监听
func (s *Server) Run() error { return s.eng.Run(s.deps.Cfg.HTTP.Addr) }

// Engine 暴露底层引擎（便于测试注入）
func (s *Server) Engine() *gin.Engine { return s.eng }
