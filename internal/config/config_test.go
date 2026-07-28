package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBootstrap(t *testing.T) {
	dir := t.TempDir()
	ef := filepath.Join(dir, ".env")
	if err := os.WriteFile(ef, []byte("# comment\n\nATLAS_PG_DSN=postgres://u:p@h:5432/db\nATLAS_NATS_URL=nats://h:4222\nATLAS_ES_ADDR=http://h:9200\n"), 0644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadBootstrapFrom(ef)
	if err != nil {
		t.Fatal(err)
	}
	if b.PGDSN != "postgres://u:p@h:5432/db" {
		t.Errorf("PGDSN=%q", b.PGDSN)
	}
	if b.NATSURL != "nats://h:4222" || b.ESAddr != "http://h:9200" {
		t.Errorf("nats/es 解析错误: %+v", b)
	}
}

func TestLoadBootstrapEnvPrecedence(t *testing.T) {
	t.Setenv("ATLAS_PG_DSN", "env-dsn")
	b, err := LoadBootstrapFrom("") // 无 .env 文件，走环境变量
	if err != nil {
		t.Fatal(err)
	}
	if b.PGDSN != "env-dsn" {
		t.Errorf("环境变量应优先，实际 %q", b.PGDSN)
	}
}
