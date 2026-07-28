//go:build integration

package config

import (
	"context"
	"os"
	"testing"

	"atlas/internal/store"
)

func TestIntegrationConfigDB(t *testing.T) {
	dsn := os.Getenv("ATLAS_PG_DSN")
	if dsn == "" {
		t.Skip("ATLAS_PG_DSN 未设置，跳过配置 DB 集成测试")
	}
	ctx := context.Background()
	st, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.RunMigrations(ctx, "migrations"); err != nil {
		t.Fatal(err)
	}
	// 清空待测表，模拟首启空库
	_, _ = st.Pool().Exec(ctx, "TRUNCATE config")
	boot, _ := LoadBootstrapFrom("")
	cfg, err := LoadFromDB(ctx, NewPoolDB(st.Pool()), boot)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Scan.DefaultMode != "connect" {
		t.Errorf("首启应播种默认值 connect, 实际 %s", cfg.Scan.DefaultMode)
	}
	if err := UpsertSection(ctx, NewPoolDB(st.Pool()), "scan", ScanConfig{DefaultMode: "syn"}); err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadFromDB(ctx, NewPoolDB(st.Pool()), boot)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Scan.DefaultMode != "syn" {
		t.Errorf("热更新后应从 DB 读到 syn, 实际 %s", cfg2.Scan.DefaultMode)
	}
}
