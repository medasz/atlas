package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 全局配置
type Config struct {
	HTTP     HTTPConfig     `yaml:"http"`
	NATS     NATSConfig     `yaml:"nats"`
	Postgres PostgresConfig `yaml:"postgres"`
	Elastic  ElasticConfig  `yaml:"elastic"`
	Scan     ScanConfig     `yaml:"scan"`
	Audit    AuditConfig    `yaml:"audit"`
	Auth     AuthConfig     `yaml:"auth"`

	path string // 配置文件路径（非导出字段，不写入 YAML，仅供 Save 写回使用）
}

type HTTPConfig struct {
	Addr string `yaml:"addr"`
}

type NATSConfig struct {
	URL string `yaml:"url"`
}

type PostgresConfig struct {
	DSN string `yaml:"dsn"`
}

type ElasticConfig struct {
	Addr  string `yaml:"addr"`
	Index string `yaml:"index"`
}

// ScanConfig 扫描默认与限速（均为手动可配置，无强制默认）
type ScanConfig struct {
	DefaultMode      string `yaml:"default_mode"`       // connect|syn|ack|fin|null|xmas
	DefaultPortRange string `yaml:"default_port_range"` // top1000|list|range|1..65535
	MaxConcurrency   int    `yaml:"max_concurrency"`    // 单实例全局最大并发（建议 500）
	PerTargetRPS     int    `yaml:"per_target_rps"`     // 每目标请求速率（建议 10）
	PortChunkSize    int    `yaml:"port_chunk_size"`    // 单 IP 端口切块大小，默认 1000

	// raw 包扫描（SYN/ACK/FIN/Null/Xmas）相关配置
	RawCaptureWindowSec int    `yaml:"raw_capture_window_sec"` // 抓包窗口（秒），默认 3
	RawRetries          int    `yaml:"raw_retries"`            // 无响应重发次数，默认 1
	RecordFilteredPorts bool   `yaml:"record_filtered_ports"`  // 是否落库 filtered（防火墙拓扑），默认 true
	RecordClosedPorts   bool   `yaml:"record_closed_ports"`    // 是否落库 closed/timeout，默认 false（防 PG 膨胀）
	InstallRstDrop      bool   `yaml:"install_rst_drop"`       // 是否尝试安装 RST-drop 规则（stealth），默认 true
	RawIface            string `yaml:"raw_iface"`              // 抓包网卡（空=自动选出口，可在界面编辑）
}

type AuditConfig struct {
	Enabled bool `yaml:"enabled"` // 审计开关
}

// AuthConfig 访问控制（MVP 单管理员口令 + 签名会话 Cookie）
type AuthConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Password string `yaml:"password"`
	Secret   string `yaml:"secret"`
}

// Load 读取 YAML 配置，环境变量可覆盖关键连接项
func Load(path string) (*Config, error) {
	cfg := defaultConfig()
	if path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		cfg.path = path
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	if v := os.Getenv("ATLAS_PG_DSN"); v != "" {
		cfg.Postgres.DSN = v
	}
	if v := os.Getenv("ATLAS_NATS_URL"); v != "" {
		cfg.NATS.URL = v
	}
	if v := os.Getenv("ATLAS_ES_ADDR"); v != "" {
		cfg.Elastic.Addr = v
	}
	if v := os.Getenv("ATLAS_HTTP_ADDR"); v != "" {
		cfg.HTTP.Addr = v
	}
	return cfg, nil
}

// Save 将当前配置写回加载时的 YAML 文件（会保留已解析的全部字段值，
// 但会丢失原文件中的注释）。若配置来自无文件路径的默认值则返回错误。
func (c *Config) Save() error {
	if c.path == "" {
		return fmt.Errorf("配置文件路径未知，无法持久化（请通过 -config 指定文件启动）")
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
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
