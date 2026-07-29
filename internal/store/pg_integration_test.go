//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestConcurrentRunMigrations 验证两个 Store 并发调用 RunMigrations 时
// pg_type_typname_nsp_index 等竞态不会出现，且每个迁移文件仅登记一次。
func TestConcurrentRunMigrations(t *testing.T) {
	dsn := os.Getenv("ATLAS_PG_DSN")
	if dsn == "" {
		t.Skip("ATLAS_PG_DSN 未设置，跳过并发迁移集成测试")
	}

	// 从测试文件路径推导 migrations 目录
	_, filename, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")

	// 生成临时 schema 名称（纯小写字母数字十六进制，安全引用无需双引号转义）
	n, _ := rand.Int(rand.Reader, big.NewInt(1<<48))
	schema := fmt.Sprintf("atlas_test_%d_%x", time.Now().UnixMilli(), n)

	// 管理连接：创建临时 schema，与 public/既有 schema 完全隔离
	adminPool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := adminPool.Exec(context.Background(),
		`CREATE SCHEMA IF NOT EXISTS "`+schema+`"`); err != nil {
		adminPool.Close()
		t.Fatalf("create schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		if _, err := adminPool.Exec(context.Background(),
			`DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`); err != nil {
			t.Logf("cleanup schema %s: %v", schema, err)
		}
		adminPool.Close()
	})

	// 解析 DSN 并为两个 Store 各自设置 search_path → 临时 schema
	cfg1, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg1.MaxConns = 3
	cfg1.ConnConfig.RuntimeParams["search_path"] = schema

	cfg2, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg2.MaxConns = 3
	cfg2.ConnConfig.RuntimeParams["search_path"] = schema

	ctx := context.Background()
	pool1, err := pgxpool.NewWithConfig(ctx, cfg1)
	if err != nil {
		t.Fatal(err)
	}
	pool2, err := pgxpool.NewWithConfig(ctx, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	s1 := &Store{pool: pool1}
	s2 := &Store{pool: pool2}
	t.Cleanup(s1.Close)
	t.Cleanup(s2.Close)

	// 共同起跑屏障：两个 goroutine 同时开始 RunMigrations
	startBarrier := make(chan struct{})
	var wg sync.WaitGroup
	var err1, err2 error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-startBarrier
		err1 = s1.RunMigrations(ctx, migrationsDir)
	}()
	go func() {
		defer wg.Done()
		<-startBarrier
		err2 = s2.RunMigrations(ctx, migrationsDir)
	}()
	close(startBarrier) // 同时释放两个 goroutine
	wg.Wait()

	if err1 != nil {
		t.Errorf("s1.RunMigrations 失败: %v", err1)
	}
	if err2 != nil {
		t.Errorf("s2.RunMigrations 失败: %v", err2)
	}
	if err1 != nil || err2 != nil {
		t.FailNow()
	}

	// 读取 migrations 目录中全部 *.up.sql 文件名作为 expected 集合
	// （与 RunMigrations 的 HasSuffix(".up.sql") 过滤保持一致）
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	expected := make(map[string]bool)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			expected[e.Name()] = true
		}
	}

	// 读取 schema_migrations 的全部 name 作为 actual 集合
	rows, err := adminPool.Query(ctx, `SELECT name FROM "`+schema+`".schema_migrations`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	actual := make(map[string]bool)
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		actual[n] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	// 断言数量相等、集合完全一致，证明每个 migration 恰好登记一次
	if len(expected) != len(actual) {
		t.Fatalf("迁移文件数 %d, 登记数 %d (expected %v, actual %v)",
			len(expected), len(actual), expected, actual)
	}
	for name := range expected {
		if !actual[name] {
			t.Errorf("迁移 %s 未登记", name)
		}
	}
	for name := range actual {
		if !expected[name] {
			t.Errorf("额外登记了非迁移文件 %s", name)
		}
	}
}
