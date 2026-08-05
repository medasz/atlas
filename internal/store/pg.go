package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/config"
	"atlas/internal/model"
)

// Store PostgreSQL 仓储（任务/漏洞/审计/黑名单/配置；资产本体已迁 Elasticsearch）
type Store struct {
	pool *pgxpool.Pool
}

// NewPostgres 建立连接池
func NewPostgres(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Pool 暴露底层连接池（供其他层直接使用）
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Close 关闭连接池
func (s *Store) Close() { s.pool.Close() }

// RunMigrations 按字典序执行 migrations 目录下所有 *.up.sql，已应用的跳过。
//
// 关键：atlas 与 atlas2 等多实例会在启动时各自执行迁移。Postgres 的
// CREATE TABLE IF NOT EXISTS 在并发下存在竞态（检查存在与创建类型非原子），
// 可能导致 pg_type_typname_nsp_index 唯一约束冲突（duplicate key）。
// 因此这里：
//  1. 从连接池获取专用连接，在连接上获取 session-level advisory 锁，
//     保证同一时刻只有一个实例在迁移，其余实例阻塞至对方解锁；
//  2. 持锁覆盖 schema_migrations 表创建、已应用版本读取与全部迁移；
//  3. 使用不受调用方 context 取消影响的短时 context 调用 pg_advisory_unlock；
//  4. 每个迁移文件的 SQL 与 schema_migrations 登记在独立事务内，
//     文件级失败不回滚已成功的其他迁移。
func (s *Store) RunMigrations(ctx context.Context, dir string) (retErr error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration conn: %w", err)
	}

	// 串行化多实例迁移；session-level 锁在显式 unlock 前持续持有，
	// 覆盖整个迁移流程（表创建、已应用读取、全部迁移执行）。
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(916873245)`); err != nil {
		conn.Release()
		return fmt.Errorf("migration lock: %w", err)
	}

	// defer unlock + release：使用 Background 派生短时 context，
	// 避免调用方 context 已取消时 unlock 无法发送。
	defer func() {
		uc, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.Exec(uc, `SELECT pg_advisory_unlock(916873245)`); err != nil && retErr == nil {
			retErr = fmt.Errorf("migration unlock: %w", err)
		}
		conn.Release()
	}()

	// 创建 schema_migrations 表（持锁期间安全）
	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name        TEXT PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// 读已应用版本（持锁期间视图一致）
	rows, err := conn.Query(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	applied := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		applied[n] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		name := e.Name()
		if applied[name] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		// 每个迁移在独立事务内执行，失败时回滚不影响其他迁移
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}

		if _, err := tx.Exec(ctx, string(data)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations(name, applied_at) VALUES($1,$2)`, name, time.Now()); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("register migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

// InsertAudit 写入审计记录（审计开关由调用方控制）
func (s *Store) InsertAudit(ctx context.Context, operator, target, taskID, action string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (operator, target, task_id, action) VALUES ($1,$2,$3,$4)`,
		operator, target, taskID, action)
	return err
}

// ListAuditLogs 分页与关键词检索审计日志
func (s *Store) ListAuditLogs(ctx context.Context, page, pageSize int, search string) ([]model.AuditLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int64
	var rows pgx.Rows
	var err error

	if search != "" {
		pattern := "%" + search + "%"
		_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE operator LIKE $1 OR target LIKE $1 OR task_id LIKE $1 OR action LIKE $1`, pattern).Scan(&total)
		rows, err = s.pool.Query(ctx, `
			SELECT id, operator, time, target, task_id, action
			FROM audit_logs
			WHERE operator LIKE $1 OR target LIKE $1 OR task_id LIKE $1 OR action LIKE $1
			ORDER BY time DESC, id DESC
			LIMIT $2 OFFSET $3`, pattern, pageSize, offset)
	} else {
		_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&total)
		rows, err = s.pool.Query(ctx, `
			SELECT id, operator, time, target, task_id, action
			FROM audit_logs
			ORDER BY time DESC, id DESC
			LIMIT $1 OFFSET $2`, pageSize, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []model.AuditLog{}
	for rows.Next() {
		var logItem model.AuditLog
		if err := rows.Scan(&logItem.ID, &logItem.Operator, &logItem.Time, &logItem.Target, &logItem.TaskID, &logItem.Action); err != nil {
			return nil, 0, err
		}
		out = append(out, logItem)
	}
	return out, total, rows.Err()
}

// AddBlacklist 新增黑名单条目（type,value 唯一，重复忽略）
func (s *Store) AddBlacklist(ctx context.Context, b model.BlacklistItem) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO blacklist (type, value, operator) VALUES ($1,$2,$3)
		ON CONFLICT (type, value) DO NOTHING`,
		b.Type, b.Value, b.Operator)
	return err
}

// ListBlacklist 列出全部黑名单条目（按创建时间倒序）
func (s *Store) ListBlacklist(ctx context.Context) ([]model.BlacklistItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT type, value, operator, created_at FROM blacklist ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.BlacklistItem{}
	for rows.Next() {
		var b model.BlacklistItem
		if err := rows.Scan(&b.Type, &b.Value, &b.Operator, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteBlacklist 删除指定黑名单条目
func (s *Store) DeleteBlacklist(ctx context.Context, typ, value string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM blacklist WHERE type=$1 AND value=$2`, typ, value)
	return err
}

// BlacklistEntries 返回全部黑名单条目原始数据（供命中判定）
func (s *Store) BlacklistEntries(ctx context.Context) ([]model.BlacklistItem, error) {
	return s.ListBlacklist(ctx)
}

// ListVulnsByHost 列出某主机（及其各端口）关联的漏洞结果
func (s *Store) ListVulnsByHost(ctx context.Context, ip string) ([]model.Vuln, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT asset_ref, kpid, cve, name, level, type, proof, status, first_found, last_verified
		FROM vulns WHERE asset_ref=$1 OR asset_ref LIKE $2
		ORDER BY level DESC, first_found DESC`, ip, ip+":%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Vuln{}
	for rows.Next() {
		var v model.Vuln
		if err := rows.Scan(&v.AssetRef, &v.KPID, &v.CVE, &v.Name, &v.Level,
			&v.Type, &v.Proof, &v.Status, &v.FirstFound, &v.LastVerified); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeleteHostMetadata removes relational data owned by a deleted host asset.
// Audit logs and scan tasks are retained as operational records.
func (s *Store) DeleteHostMetadata(ctx context.Context, ip string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM vulns WHERE asset_ref=$1 OR asset_ref LIKE $2`, ip, ip+":%"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM asset_history WHERE ip=$1`, ip); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ip_survivals WHERE ip=$1`, ip); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteVulnsByAsset removes vulnerability results tied exactly to one asset.
func (s *Store) DeleteVulnsByAsset(ctx context.Context, assetRef string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM vulns WHERE asset_ref=$1`, assetRef)
	return err
}

// DeletePortMetadata removes relational data for one port and reconciles the host summary.
func (s *Store) DeletePortMetadata(ctx context.Context, ip string, port, remainingOpen int, removeHost bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	assetRef := ip + ":" + strconv.Itoa(port)
	if _, err := tx.Exec(ctx, `DELETE FROM vulns WHERE asset_ref=$1`, assetRef); err != nil {
		return err
	}
	if removeHost {
		if _, err := tx.Exec(ctx, `DELETE FROM vulns WHERE asset_ref=$1 OR asset_ref LIKE $2`, ip, ip+":%"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM asset_history WHERE ip=$1`, ip); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM ip_survivals WHERE ip=$1`, ip); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `DELETE FROM asset_history WHERE ip=$1 AND port=$2`, ip, port); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE ip_survivals SET open_ports=$2 WHERE ip=$1`, ip, remainingOpen); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CreateTask 持久化任务
func (s *Store) CreateTask(ctx context.Context, t model.Task) error {
	scope, _ := json.Marshal(t.Scope)
	sched, _ := json.Marshal(t.Schedule)
	rl, _ := json.Marshal(t.RateLimit)
	scanCfg, _ := json.Marshal(t.ScanConfig)
	prog, _ := json.Marshal(t.Progress)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tasks (id, kind, scope, schedule, rate_limit, scan_config, status, progress)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		t.ID, t.Kind, scope, sched, rl, scanCfg, t.Status, prog)
	return err
}

// GetTask 按 ID 读取任务
func (s *Store) GetTask(ctx context.Context, id string) (model.Task, error) {
	var t model.Task
	var scope, sched, rl, scanCfg, prog []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, kind, scope, schedule, rate_limit, scan_config, status, progress, created_at
		FROM tasks WHERE id=$1`, id).
		Scan(&t.ID, &t.Kind, &scope, &sched, &rl, &scanCfg, &t.Status, &prog, &t.CreatedAt)
	if err != nil {
		return t, err
	}
	_ = json.Unmarshal(scope, &t.Scope)
	_ = json.Unmarshal(sched, &t.Schedule)
	_ = json.Unmarshal(rl, &t.RateLimit)
	_ = json.Unmarshal(scanCfg, &t.ScanConfig)
	_ = json.Unmarshal(prog, &t.Progress)
	return t, nil
}

// ListTasks 列出全部任务（倒序）
func (s *Store) ListTasks(ctx context.Context) ([]model.Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, kind, scope, schedule, rate_limit, scan_config, status, progress, created_at
		FROM tasks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Task{}
	for rows.Next() {
		var t model.Task
		var scope, sched, rl, scanCfg, prog []byte
		if err := rows.Scan(&t.ID, &t.Kind, &scope, &sched, &rl, &scanCfg, &t.Status, &prog, &t.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(scope, &t.Scope)
		_ = json.Unmarshal(sched, &t.Schedule)
		_ = json.Unmarshal(rl, &t.RateLimit)
		_ = json.Unmarshal(scanCfg, &t.ScanConfig)
		_ = json.Unmarshal(prog, &t.Progress)
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTask 删除任务及其全部子项
func (s *Store) DeleteTask(ctx context.Context, id string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM tasks WHERE id=$1`, id); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM task_items WHERE task_id=$1`, id)
	return err
}

// UpdateTaskStatus 更新任务状态
func (s *Store) UpdateTaskStatus(ctx context.Context, id string, status int) error {
	_, err := s.pool.Exec(ctx, `UPDATE tasks SET status=$2 WHERE id=$1`, id, status)
	return err
}

// UpdateTaskProgress 更新任务进度计数
func (s *Store) UpdateTaskProgress(ctx context.Context, id string, total, done int) error {
	prog, _ := json.Marshal(map[string]int{"total": total, "done": done})
	_, err := s.pool.Exec(ctx, `UPDATE tasks SET progress=$2 WHERE id=$1`, id, prog)
	return err
}

// UpsertTaskItem 写入任务子项（冲突更新状态/结果）
func (s *Store) UpsertTaskItem(ctx context.Context, item model.TaskItem) error {
	res, _ := json.Marshal(item.Result)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO task_items (task_id, target, ports, status, result)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (task_id, target, ports) DO UPDATE SET status=EXCLUDED.status, result=EXCLUDED.result`,
		item.TaskID, item.Target, item.Ports, item.Status, res)
	return err
}

// ListTaskItems 列出任务子项，statusFilter 为 nil 时返回全部
func (s *Store) ListTaskItems(ctx context.Context, taskID string, statusFilter *int) ([]model.TaskItem, error) {
	var rows pgx.Rows
	var err error
	if statusFilter == nil {
		rows, err = s.pool.Query(ctx, `SELECT task_id, target, ports, status, result FROM task_items WHERE task_id=$1`, taskID)
	} else {
		rows, err = s.pool.Query(ctx, `SELECT task_id, target, ports, status, result FROM task_items WHERE task_id=$1 AND status=$2`, taskID, *statusFilter)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.TaskItem{}
	for rows.Next() {
		var it model.TaskItem
		var res []byte
		if err := rows.Scan(&it.TaskID, &it.Target, &it.Ports, &it.Status, &res); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(res, &it.Result)
		out = append(out, it)
	}
	return out, rows.Err()
}

// MarkItemDone 标记子项完成并写入结果
func (s *Store) MarkItemDone(ctx context.Context, taskID, target, ports string, result map[string]any) error {
	return s.UpsertTaskItem(ctx, model.TaskItem{TaskID: taskID, Target: target, Ports: ports, Status: model.TaskItemDone, Result: result})
}

// CountTaskItems 统计子项总数与已完成数
func (s *Store) CountTaskItems(ctx context.Context, taskID string) (total, done int, err error) {
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM task_items WHERE task_id=$1`, taskID).Scan(&total)
	if err != nil {
		return
	}
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM task_items WHERE task_id=$1 AND status=$2`, taskID, model.TaskItemDone).Scan(&done)
	return
}

// UpsertVuln 写入/更新漏洞结果（按 asset_ref + kpid 冲突更新）
func (s *Store) UpsertVuln(ctx context.Context, v model.Vuln) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO vulns (asset_ref, kpid, cve, name, level, type, proof, status, first_found, last_verified)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now(),now())
		ON CONFLICT (asset_ref, kpid) DO UPDATE SET
			cve=EXCLUDED.cve, name=EXCLUDED.name, level=EXCLUDED.level, type=EXCLUDED.type,
			proof=EXCLUDED.proof, status=EXCLUDED.status, last_verified=now()`,
		v.AssetRef, v.KPID, v.CVE, v.Name, v.Level, v.Type, v.Proof, v.Status)
	return err
}

// ListVulns 列出漏洞结果；assetRef 非空时按资产过滤
func (s *Store) ListVulns(ctx context.Context, assetRef string) ([]model.Vuln, error) {
	var rows pgx.Rows
	var err error
	if assetRef != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT asset_ref, kpid, cve, name, level, type, proof, status, first_found, last_verified
			FROM vulns WHERE asset_ref=$1 ORDER BY level DESC, first_found DESC`, assetRef)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT asset_ref, kpid, cve, name, level, type, proof, status, first_found, last_verified
			FROM vulns ORDER BY level DESC, first_found DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Vuln{}
	for rows.Next() {
		var v model.Vuln
		if err := rows.Scan(&v.AssetRef, &v.KPID, &v.CVE, &v.Name, &v.Level, &v.Type, &v.Proof, &v.Status, &v.FirstFound, &v.LastVerified); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpsertTemplate 持久化检测模板
func (s *Store) UpsertTemplate(ctx context.Context, id, content string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO templates (id, kind, content, enabled) VALUES ($1,'yaml',$2,true)
		ON CONFLICT (id) DO UPDATE SET content=EXCLUDED.content, enabled=true`,
		id, content)
	return err
}

// ListTemplates 列出已持久化模板（返回 id 与内容）
func (s *Store) ListTemplates(ctx context.Context) ([]struct {
	ID      string
	Content string
}, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, content FROM templates WHERE enabled`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []struct {
		ID      string
		Content string
	}{}
	for rows.Next() {
		var r struct {
			ID      string
			Content string
		}
		if err := rows.Scan(&r.ID, &r.Content); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertConfigSection 写入单配置段（调用 config.UpsertSection 展平为单项 KV 入库）
func (s *Store) UpsertConfigSection(ctx context.Context, section, valueJSON string) error {
	var obj any
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return config.UpsertSection(ctx, config.NewPoolDB(s.pool), section, valueJSON)
	}
	return config.UpsertSection(ctx, config.NewPoolDB(s.pool), section, obj)
}

// UpsertIPLifecycle 更新或插入主机在线打卡与生命周期状态
func (s *Store) UpsertIPLifecycle(ctx context.Context, ip string, isV6 bool, openPorts int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ip_survivals (ip, open_ports, is_ipv6, first_seen, last_seen)
		VALUES ($1, $2, $3, now(), now())
		ON CONFLICT (ip) DO UPDATE SET
			open_ports = EXCLUDED.open_ports,
			is_ipv6    = EXCLUDED.is_ipv6,
			last_seen  = EXCLUDED.last_seen`,
		ip, openPorts, isV6)
	if err != nil {
		return fmt.Errorf("upsert ip lifecycle %s: %w", ip, err)
	}
	return nil
}
