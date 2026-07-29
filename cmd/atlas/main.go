package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"atlas/internal/audit"
	"atlas/internal/blacklist"
	"atlas/internal/config"
	"atlas/internal/fingerprint"
	"atlas/internal/queue"
	"atlas/internal/ratelimit"
	"atlas/internal/esasset"
	"atlas/internal/scan"
	"atlas/internal/server"
	"atlas/internal/store"
	"atlas/internal/task"
	"atlas/internal/vuln"
)

func main() {
	envFile := flag.String("envfile", "", "path to .env file for connection bootstrap (env vars override)")
	migrationsDir := flag.String("migrations", "migrations", "path to sql migrations dir")
	rulesPath := flag.String("rules", "configs/fingerprint-rules.yaml", "path to fingerprint rules yaml")
	webDir := flag.String("webdir", "", "path to built frontend (web/dist); if set, serve SPA")
	templatesDir := flag.String("templates", "configs/templates", "path to vuln template yaml dir")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 引导连接参数（.env / 环境变量），不进 DB
	boot, err := config.LoadBootstrapFrom(*envFile)
	if err != nil {
		log.Fatalf("load bootstrap: %v", err)
	}

	// PostgreSQL
	st, err := store.NewPostgres(ctx, boot.PGDSN)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer st.Close()

	if err := st.RunMigrations(ctx, *migrationsDir); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Println("migrations applied")

	// 配置从 DB 读取（首启自动播种默认值）
	cfg, err := config.LoadFromDB(ctx, config.NewPoolDB(st.Pool()), boot)
	if err != nil {
		log.Fatalf("load config from db: %v", err)
	}

	// Elasticsearch 成为资产唯一存储（不再回退 PG 资产）
	es := store.NewES(cfg.Elastic.Addr, cfg.Elastic.Index)
	if err := es.CreateIndex(ctx); err != nil {
		log.Printf("elasticsearch init warning: %v", err)
	}
	// 资产存储：ES 唯一实现
	assetStore := esasset.New(es)

	// 审计（开关由配置控制）
	auditor := audit.New(st, cfg.Audit.Enabled)

	// 限速（全局 + 每目标令牌桶）
	limiter := ratelimit.New(cfg.Scan.MaxConcurrency, cfg.Scan.PerTargetRPS)

	// 指纹识别（自研规则热加载；规则文件不存在则仅用社区库）
	var fp *fingerprint.Service
	if _, statErr := os.Stat(*rulesPath); statErr == nil {
		if fp, err = fingerprint.New(*rulesPath); err != nil {
			log.Fatalf("fingerprint: %v", err)
		}
		log.Printf("fingerprint rules loaded: %s", *rulesPath)
	} else {
		fp, _ = fingerprint.New("")
		log.Println("fingerprint: community library only (no custom rules)")
	}

	// NATS（跨实例分发；连接失败则单实例进程内运行）
	var q *queue.Queue
	if nc, err := queue.New(cfg.NATS.URL); err != nil {
		log.Printf("nats unavailable, running in-process: %v", err)
	} else {
		q = nc
		defer q.Close()
		log.Println("nats connected")
	}

	// 黑名单服务
	bl := blacklist.New(st, auditor)

	// 默认端口列表（供任务按块建子项使用）
	var defaultPorts []int
	if ps, perr := scan.ParsePortSpec(cfg.Scan.DefaultPortRange); perr == nil && len(ps) > 0 {
		defaultPorts = ps
	}

	// 任务调度服务（默认占位处理器，Issue #4 注入真实探测）
	taskSvc := task.New(st, q, auditor, bl, limiter, cfg.Scan.MaxConcurrency, defaultPorts, cfg.Scan.PortChunkSize)

	// 资产探测引擎（Issue #4）：注入为资产扫描处理器，传入扫描配置（模式 + raw 参数）
	scanner := scan.New(assetStore, limiter, defaultPorts, fp, cfg.Scan)
	taskSvc.SetProcessor(scanner)

	// 漏洞检测引擎（Issue #10~#13）：加载目录模板 + 已持久化模板
	vulnEngine, err := vuln.New(st, limiter, *templatesDir)
	if err != nil {
		log.Fatalf("vuln engine: %v", err)
	}
	if dbTpls, derr := st.ListTemplates(ctx); derr == nil {
		for _, row := range dbTpls {
			if t, perr := vuln.ParseTemplate([]byte(row.Content)); perr == nil {
				vulnEngine.AddTemplate(t)
			}
		}
	}
	taskSvc.SetVulnProcessor(vulnEngine)
	log.Printf("vuln templates loaded: %d", len(vulnEngine.Templates()))

	if err := taskSvc.RegisterWorker(); err != nil {
		log.Fatalf("register worker: %v", err)
	}

	srv := server.New(server.Deps{
		Cfg:        cfg,
		Store:      st,
		Asset:      assetStore,
		Queue:      q,
		Audit:      auditor,
		Rate:       limiter,
		Blacklist:  bl,
		Task:       taskSvc,
		Scanner:    scanner,
		Fingerprint: fp,
		Vuln:       vulnEngine,
		WebDir:     *webDir,
	})

	go func() {
		log.Printf("atlas listening on %s", cfg.HTTP.Addr)
		if err := srv.Run(); err != nil {
			log.Fatalf("server: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for {
		s := <-sig
		switch s {
		case syscall.SIGHUP:
			if err := fp.Reload(); err != nil {
				log.Printf("fingerprint reload failed: %v", err)
			} else {
				log.Println("fingerprint rules reloaded")
			}
		default:
			log.Println("shutting down")
			return
		}
	}
}
