package config

import (
	"fmt"
	"os"
	"path/filepath"

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
	DefaultMode      string `yaml:"default_mode"`       // connect|syn|fin|null|xmas|udp|ack
	DefaultPortRange string `yaml:"default_port_range"` // top1000|list|range|1..65535
	MaxConcurrency   int    `yaml:"max_concurrency"`    // 单实例全局最大并发（建议 500）
	PerTargetRPS     int    `yaml:"per_target_rps"`     // 每目标请求速率（建议 10）
	PortChunkSize    int    `yaml:"port_chunk_size"`    // 单 IP 端口切块大小，默认 1000
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
			DefaultMode:      "connect",
			DefaultPortRange: "top1000",
			MaxConcurrency:   500,
			PerTargetRPS:     10,
			PortChunkSize:    1000,
		},
		Audit: AuditConfig{Enabled: true},
		Auth:  AuthConfig{Enabled: true, Password: "admin", Secret: "atlas-dev-secret-change-me"},
	}
}
