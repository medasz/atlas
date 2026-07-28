package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// --- DB 读写层（fakeDB，无需真库） ---

type fakeRow struct{ val string; err error }

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for _, d := range dest {
		switch p := d.(type) {
		case *string:
			*p = r.val
		case *int:
			n, err := strconv.Atoi(r.val)
			if err != nil {
				return err
			}
			*p = n
		default:
			return fmt.Errorf("unsupported scan dest %T", d)
		}
	}
	return nil
}

type fakeDB struct{ rows map[string]string }

func (d *fakeDB) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	if len(args) >= 2 {
		if k, ok := args[0].(string); ok {
			if v, ok := args[1].(string); ok {
				d.rows[k] = v
			}
		}
	}
	return pgconn.CommandTag{}, nil
}

func (d *fakeDB) QueryRow(_ context.Context, _ string, args ...any) Row {
	if len(args) == 0 {
		// 无参数查询（如 SELECT count(*)）返回当前配置段数量
		return fakeRow{val: strconv.Itoa(len(d.rows))}
	}
	key, _ := args[0].(string)
	if v, ok := d.rows[key]; ok {
		return fakeRow{val: v}
	}
	return fakeRow{err: pgx.ErrNoRows}
}

func TestConfigSeedAndLoad(t *testing.T) {
	db := &fakeDB{rows: map[string]string{}}
	if err := EnsureSeeded(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if len(db.rows) != 4 {
		t.Fatalf("应播种 4 段，实际 %d", len(db.rows))
	}
	cfg, err := LoadFromDB(context.Background(), db, &Bootstrap{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Scan.DefaultMode != "connect" {
		t.Errorf("默认模式应为 connect, 实际 %s", cfg.Scan.DefaultMode)
	}
	// 热更新一段后重读应生效
	if err := UpsertSection(context.Background(), db, "scan", ScanConfig{DefaultMode: "syn", RawIface: "eth0"}); err != nil {
		t.Fatal(err)
	}
	cfg2, _ := LoadFromDB(context.Background(), db, &Bootstrap{})
	if cfg2.Scan.DefaultMode != "syn" || cfg2.Scan.RawIface != "eth0" {
		t.Errorf("热更新后未生效: %+v", cfg2.Scan)
	}
}
