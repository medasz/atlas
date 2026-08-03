package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
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

// Rows 是 Query 返回多行的最小接口
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

// DB 解耦存储层的最小接口；*pgxpool.Pool 经 PoolDB 适配后满足。
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
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

func (p PoolDB) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := p.pool.Query(ctx, sql, args...)
	return rows, err
}

const upsertConfigSQL = `INSERT INTO config(key,value,updated_at) VALUES($1,$2,now())
	ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`

// configToKV 将 Config 结构体转为下划线 KEY 的字符串 Map
func configToKV(cfg *Config) map[string]string {
	return map[string]string{
		"scan_default_mode":           cfg.Scan.DefaultMode,
		"scan_default_port_range":    cfg.Scan.DefaultPortRange,
		"scan_max_concurrency":      fmt.Sprintf("%d", cfg.Scan.MaxConcurrency),
		"scan_per_target_rps":         fmt.Sprintf("%d", cfg.Scan.PerTargetRPS),
		"scan_port_chunk_size":        fmt.Sprintf("%d", cfg.Scan.PortChunkSize),
		"scan_raw_capture_window_sec": fmt.Sprintf("%d", cfg.Scan.RawCaptureWindowSec),
		"scan_raw_retries":            fmt.Sprintf("%d", cfg.Scan.RawRetries),
		"scan_record_filtered_ports":  fmt.Sprintf("%t", cfg.Scan.RecordFilteredPorts),
		"scan_record_closed_ports":    fmt.Sprintf("%t", cfg.Scan.RecordClosedPorts),
		"scan_install_rst_drop":       fmt.Sprintf("%t", cfg.Scan.InstallRstDrop),
		"scan_raw_iface":              cfg.Scan.RawIface,
		"auth_enabled":                fmt.Sprintf("%t", cfg.Auth.Enabled),
		"auth_password":               cfg.Auth.Password,
		"auth_secret":                 cfg.Auth.Secret,
		"audit_enabled":               fmt.Sprintf("%t", cfg.Audit.Enabled),
	}
}

// applyKVToConfig 将 KV 字典解析填入 Config
func applyKVToConfig(cfg *Config, kv map[string]string) {
	for k, v := range kv {
		switch k {
		case "scan_default_mode":
			cfg.Scan.DefaultMode = v
		case "scan_default_port_range":
			cfg.Scan.DefaultPortRange = v
		case "scan_max_concurrency":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Scan.MaxConcurrency = n
			}
		case "scan_per_target_rps":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Scan.PerTargetRPS = n
			}
		case "scan_port_chunk_size":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Scan.PortChunkSize = n
			}
		case "scan_raw_capture_window_sec":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Scan.RawCaptureWindowSec = n
			}
		case "scan_raw_retries":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Scan.RawRetries = n
			}
		case "scan_record_filtered_ports":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.Scan.RecordFilteredPorts = b
			}
		case "scan_record_closed_ports":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.Scan.RecordClosedPorts = b
			}
		case "scan_install_rst_drop":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.Scan.InstallRstDrop = b
			}
		case "scan_raw_iface":
			cfg.Scan.RawIface = v
		case "auth_enabled":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.Auth.Enabled = b
			}
		case "auth_password":
			cfg.Auth.Password = v
		case "auth_secret":
			cfg.Auth.Secret = v
		case "audit_enabled":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.Audit.Enabled = b
			}
		}
	}
}

// sectionToKV 将某模块段转为下划线 KV
func sectionToKV(section string, v any) (map[string]string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("序列化配置段 %s: %w", section, err)
	}
	out := make(map[string]string)
	switch section {
	case "scan":
		var sc ScanConfig
		if err := json.Unmarshal(b, &sc); err != nil {
			return nil, err
		}
		out["scan_default_mode"] = sc.DefaultMode
		out["scan_default_port_range"] = sc.DefaultPortRange
		out["scan_max_concurrency"] = fmt.Sprintf("%d", sc.MaxConcurrency)
		out["scan_per_target_rps"] = fmt.Sprintf("%d", sc.PerTargetRPS)
		out["scan_port_chunk_size"] = fmt.Sprintf("%d", sc.PortChunkSize)
		out["scan_raw_capture_window_sec"] = fmt.Sprintf("%d", sc.RawCaptureWindowSec)
		out["scan_raw_retries"] = fmt.Sprintf("%d", sc.RawRetries)
		out["scan_record_filtered_ports"] = fmt.Sprintf("%t", sc.RecordFilteredPorts)
		out["scan_record_closed_ports"] = fmt.Sprintf("%t", sc.RecordClosedPorts)
		out["scan_install_rst_drop"] = fmt.Sprintf("%t", sc.InstallRstDrop)
		out["scan_raw_iface"] = sc.RawIface
	case "auth":
		var ac AuthConfig
		if err := json.Unmarshal(b, &ac); err != nil {
			return nil, err
		}
		out["auth_enabled"] = fmt.Sprintf("%t", ac.Enabled)
		out["auth_password"] = ac.Password
		out["auth_secret"] = ac.Secret
	case "audit":
		var ac AuditConfig
		if err := json.Unmarshal(b, &ac); err != nil {
			return nil, err
		}
		out["audit_enabled"] = fmt.Sprintf("%t", ac.Enabled)
	case "http":
		var hc HTTPConfig
		if err := json.Unmarshal(b, &hc); err != nil {
			return nil, err
		}
		out["http_addr"] = hc.Addr
	default:
		return nil, fmt.Errorf("未知配置段: %s", section)
	}
	return out, nil
}

// UpsertSection 将某配置段展平为 KV 后 upsert 入库。
func UpsertSection(ctx context.Context, db DB, key string, v any) error {
	kvMap, err := sectionToKV(key, v)
	if err != nil {
		return err
	}
	for k, val := range kvMap {
		if _, err := db.Exec(ctx, upsertConfigSQL, k, val); err != nil {
			return fmt.Errorf("写入配置项 %s: %w", k, err)
		}
	}
	return nil
}

// EnsureSeeded 表空时按 defaultConfig 播种单项 KV；非空则跳过。
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
	def := defaultConfig()
	kv := configToKV(def)
	for k, v := range kv {
		if _, err := db.Exec(ctx, upsertConfigSQL, k, v); err != nil {
			return fmt.Errorf("播种配置项 %s: %w", k, err)
		}
	}
	return nil
}

// LoadFromDB 确保已播种并读所有单项 KV 进 *Config；连接段由 boot 填充。
func LoadFromDB(ctx context.Context, db DB, boot *Bootstrap) (*Config, error) {
	if err := EnsureSeeded(ctx, db); err != nil {
		return nil, err
	}
	cfg := defaultConfig()
	rows, err := db.Query(ctx, "SELECT key, value FROM config")
	if err != nil {
		return nil, fmt.Errorf("查询全量配置: %w", err)
	}
	defer rows.Close()

	kv := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("扫描配置行: %w", err)
		}
		kv[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历配置行: %w", err)
	}

	applyKVToConfig(cfg, kv)

	cfg.Postgres.DSN = boot.PGDSN
	cfg.NATS.URL = boot.NATSURL
	cfg.Elastic.Addr = boot.ESAddr
	if boot.HTTPAddr != "" {
		cfg.HTTP.Addr = boot.HTTPAddr
	}
	return cfg, nil
}

