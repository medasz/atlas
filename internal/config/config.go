package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config 全局配置（连接段由 .env/环境变量引导，业务段由 DB 动态读写）
type Config struct {
	HTTP     HTTPConfig     `json:"http"`
	NATS     NATSConfig     `json:"nats"`
	Postgres PostgresConfig `json:"postgres"`
	Elastic  ElasticConfig  `json:"elastic"`
	Scan     ScanConfig     `json:"scan"`
	Audit    AuditConfig    `json:"audit"`
	Auth     AuthConfig     `json:"auth"`
}

type HTTPConfig struct {
	Addr string `json:"addr"`
}

type NATSConfig struct {
	URL string `json:"url"`
}

type PostgresConfig struct {
	DSN string `json:"dsn"`
}

type ElasticConfig struct {
	Addr  string `json:"addr"`
	Index string `json:"index"`
}

// ScanConfig 扫描默认与限速（均为手动可配置，无强制默认）
type ScanConfig struct {
	DefaultMode      string `json:"default_mode"`       // connect|syn|ack|fin|null|xmas
	DefaultPortRange string `json:"default_port_range"` // top1000|list|range|1..65535
	MaxConcurrency   int    `json:"max_concurrency"`    // 单实例全局最大并发（建议 500）
	PerTargetRPS     int    `json:"per_target_rps"`     // 每目标请求速率（建议 10）
	PortChunkSize    int    `json:"port_chunk_size"`    // 单 IP 端口切块大小，默认 1000

	// raw 包扫描（SYN/ACK/FIN/Null/Xmas）相关配置
	RawCaptureWindowSec int    `json:"raw_capture_window_sec"` // 抓包窗口（秒），默认 3
	RawRetries          int    `json:"raw_retries"`            // 无响应重发次数，默认 1
	RecordFilteredPorts bool   `json:"record_filtered_ports"`  // 是否落库 filtered（防火墙拓扑），默认 true
	RecordClosedPorts   bool   `json:"record_closed_ports"`    // 是否落库 closed/timeout，默认 false（防 PG 膨胀）
	InstallRstDrop      bool   `json:"install_rst_drop"`       // 是否尝试安装 RST-drop 规则（stealth），默认 true
	RawIface            string `json:"raw_iface"`              // 抓包网卡（空=自动选出口，可在界面编辑）
}

type AuditConfig struct {
	Enabled bool `json:"enabled"` // 审计开关
}

// AuthConfig 访问控制（MVP 单管理员口令 + 签名会话 Cookie）
type AuthConfig struct {
	Enabled  bool   `json:"enabled"`
	Password string `json:"password"`
	Secret   string `json:"secret"`
}

// Bootstrap 连接引导参数（来自 .env / 环境变量，不进 DB）。
// 这些参数无法从 DB 读取（需要先连 DB），故由 .env/环境变量提供。
type Bootstrap struct {
	PGDSN    string
	NATSURL  string
	ESAddr   string
	HTTPAddr string
}

// LoadBootstrapFrom 解析可选 .env 文件；环境变量优先，文件作兜底。
// 解析出的连接参数仅用于引导连接与首启播种，不持久化到 DB。
func LoadBootstrapFrom(envFile string) (*Bootstrap, error) {
	if envFile != "" {
		if m, err := parseEnvFile(envFile); err == nil {
			apply := func(k, env string) {
				if os.Getenv(env) == "" {
					if v, ok := m[k]; ok {
						_ = os.Setenv(env, v)
					}
				}
			}
			apply("ATLAS_PG_DSN", "ATLAS_PG_DSN")
			apply("ATLAS_NATS_URL", "ATLAS_NATS_URL")
			apply("ATLAS_ES_ADDR", "ATLAS_ES_ADDR")
			apply("ATLAS_HTTP_ADDR", "ATLAS_HTTP_ADDR")
		}
	}
	b := &Bootstrap{}
	b.PGDSN = envOr("ATLAS_PG_DSN", "postgres://postgres:postgres@127.0.0.1:5432/atlas?sslmode=disable")
	b.NATSURL = envOr("ATLAS_NATS_URL", "nats://127.0.0.1:4222")
	b.ESAddr = envOr("ATLAS_ES_ADDR", "http://127.0.0.1:9200")
	b.HTTPAddr = os.Getenv("ATLAS_HTTP_ADDR") // 空则后续由 DB http 段兜底
	return b, nil
}

// LoadBootstrap 以默认查找路径调用（实际部署由 main 传 --envfile）。
func LoadBootstrap() (*Bootstrap, error) { return LoadBootstrapFrom("") }

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// parseEnvFile 解析 KEY=VALUE 行，忽略 # 注释与空行（零依赖）。
func parseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		m[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
	}
	return m, nil
}

func defaultConfig() *Config {
	return &Config{
		HTTP:     HTTPConfig{Addr: ":8080"},
		NATS:     NATSConfig{URL: "nats://127.0.0.1:4222"},
		Postgres: PostgresConfig{DSN: "postgres://postgres:postgres@127.0.0.1:5432/atlas?sslmode=disable"},
		Elastic:  ElasticConfig{Addr: "http://127.0.0.1:9200", Index: "assets"},
		Scan: ScanConfig{
			DefaultMode:         "connect",
			DefaultPortRange:    "top1000",
			MaxConcurrency:      500,
			PerTargetRPS:        10,
			PortChunkSize:       1000,
			RawCaptureWindowSec: 3,
			RawRetries:          1,
			RecordFilteredPorts: true,
			RecordClosedPorts:   false,
			InstallRstDrop:      true,
			RawIface:            "",
		},
		Audit: AuditConfig{Enabled: true},
		Auth:  AuthConfig{Enabled: true, Password: "admin", Secret: "atlas-dev-secret-change-me"},
	}
}

// Row 是 QueryRow 返回行的最小接口（pgx.Row 天然满足）。
type Row interface{ Scan(dest ...any) error }

// DB 解耦存储层的最小接口；*pgxpool.Pool 经 PoolDB 适配后满足。
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
}

// PoolDB 将 *pgxpool.Pool 适配为 config.DB（pgx.Row 已实现 Row.Scan）。
type PoolDB struct {
	pool *pgxpool.Pool
}

// NewPoolDB 构造 DB 适配器。
func NewPoolDB(p *pgxpool.Pool) PoolDB { return PoolDB{pool: p} }

func (p PoolDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return p.pool.Exec(ctx, sql, args...)
}

func (p PoolDB) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return p.pool.QueryRow(ctx, sql, args...)
}

const upsertConfigSQL = `INSERT INTO config(key,value,updated_at) VALUES($1,$2,now())
	ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`

// UpsertSection 将某配置段 JSON 序列化后 upsert 入库。
func UpsertSection(ctx context.Context, db DB, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("序列化配置段 %s: %w", key, err)
	}
	if _, err := db.Exec(ctx, upsertConfigSQL, key, string(b)); err != nil {
		return fmt.Errorf("写入配置段 %s: %w", key, err)
	}
	return nil
}

// EnsureSeeded 表空时按 defaultConfig 播种四段；非空则跳过。
func EnsureSeeded(ctx context.Context, db DB) error {
	var n int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM config").Scan(&n); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			n = 0
		} else {
			return fmt.Errorf("统计配置行: %w", err)
		}
	}
	if n > 0 {
		return nil
	}
	seed := map[string]any{
		"scan":  defaultConfig().Scan,
		"audit": defaultConfig().Audit,
		"auth":  defaultConfig().Auth,
		"http":  defaultConfig().HTTP,
	}
	for k, v := range seed {
		if err := UpsertSection(ctx, db, k, v); err != nil {
			return err
		}
	}
	return nil
}

// LoadFromDB 确保已播种并读四段进 *Config；连接段由 boot 填充。
func LoadFromDB(ctx context.Context, db DB, boot *Bootstrap) (*Config, error) {
	if err := EnsureSeeded(ctx, db); err != nil {
		return nil, err
	}
	cfg := defaultConfig()
	for _, key := range []string{"scan", "audit", "auth", "http"} {
		row := db.QueryRow(ctx, "SELECT value FROM config WHERE key=$1", key)
		var val string
		if err := row.Scan(&val); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, fmt.Errorf("读取配置段 %s: %w", key, err)
		}
		switch key {
		case "scan":
			if err := json.Unmarshal([]byte(val), &cfg.Scan); err != nil {
				return nil, fmt.Errorf("解析 scan: %w", err)
			}
		case "audit":
			if err := json.Unmarshal([]byte(val), &cfg.Audit); err != nil {
				return nil, fmt.Errorf("解析 audit: %w", err)
			}
		case "auth":
			if err := json.Unmarshal([]byte(val), &cfg.Auth); err != nil {
				return nil, fmt.Errorf("解析 auth: %w", err)
			}
		case "http":
			if err := json.Unmarshal([]byte(val), &cfg.HTTP); err != nil {
				return nil, fmt.Errorf("解析 http: %w", err)
			}
		}
	}
	cfg.Postgres.DSN = boot.PGDSN
	cfg.NATS.URL = boot.NATSURL
	cfg.Elastic.Addr = boot.ESAddr
	if cfg.HTTP.Addr == "" && boot.HTTPAddr != "" {
		cfg.HTTP.Addr = boot.HTTPAddr
	}
	return cfg, nil
}
