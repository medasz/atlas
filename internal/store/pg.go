package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/model"
)

// Store PostgreSQL 仓储（资产/任务/漏洞/审计/黑名单等）
type Store struct {
	pool *pgxpool.Pool
	es   *ESClient

	mu        sync.Mutex
	pendingES []pendingDoc // ES 索引失败待补队列
}

// pendingDoc ES 待补文档
type pendingDoc struct {
	id  string
	doc map[string]any
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

// SetSearch 启用 Elasticsearch 同步（nil 表示仅用 PostgreSQL）
func (s *Store) SetSearch(es *ESClient) { s.es = es }

// indexAsset 写入 ES；失败则将文档入待补队列并标记 es_pending（资产不丢）
func (s *Store) indexAsset(ctx context.Context, id string, doc map[string]any) {
	if err := s.es.IndexAsset(ctx, id, doc); err == nil {
		return
	}
	s.mu.Lock()
	s.pendingES = append(s.pendingES, pendingDoc{id: id, doc: doc})
	s.mu.Unlock()
	if doc["doc_type"] == "host" {
		_, _ = s.pool.Exec(ctx, `UPDATE hosts SET es_pending=true WHERE ip=$1`, doc["ip"])
	} else {
		_, _ = s.pool.Exec(ctx, `UPDATE ports SET es_pending=true WHERE ip=$1 AND port=$2`, doc["ip"], doc["port"])
	}
}

// FlushPendingES 重试 ES 待补文档，成功后清除 es_pending 标记
func (s *Store) FlushPendingES(ctx context.Context) {
	s.mu.Lock()
	pending := s.pendingES
	s.pendingES = nil
	s.mu.Unlock()
	for _, p := range pending {
		if err := s.es.IndexAsset(ctx, p.id, p.doc); err != nil {
			s.mu.Lock()
			s.pendingES = append(s.pendingES, p)
			s.mu.Unlock()
			continue
		}
		if p.doc["doc_type"] == "host" {
			_, _ = s.pool.Exec(ctx, `UPDATE hosts SET es_pending=false WHERE ip=$1`, p.doc["ip"])
		} else {
			_, _ = s.pool.Exec(ctx, `UPDATE ports SET es_pending=false WHERE ip=$1 AND port=$2`, p.doc["ip"], p.doc["port"])
		}
	}
}

// Close 关闭连接池
func (s *Store) Close() { s.pool.Close() }

// RunMigrations 按字典序执行 migrations 目录下所有 *.up.sql，已应用的跳过
func (s *Store) RunMigrations(ctx context.Context, dir string) error {
	if err := s.ensureMigrationsTable(ctx); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	applied, err := s.appliedMigrations(ctx)
	if err != nil {
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
			return err
		}
		if _, err := s.pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO schema_migrations(name, applied_at) VALUES($1,$2)`, name, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureMigrationsTable(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

func (s *Store) appliedMigrations(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		m[n] = true
	}
	return m, rows.Err()
}

// UpsertHost 写入/更新主机资产（按 ip 冲突更新），并同步 ES
func (s *Store) UpsertHost(ctx context.Context, h model.Host) error {
	geo, _ := json.Marshal(h.Geo)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO hosts (ip, asn, org, geo, os, is_ipv6, open_ports, first_seen, last_seen)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (ip) DO UPDATE SET
			asn=EXCLUDED.asn, org=EXCLUDED.org, geo=EXCLUDED.geo, os=EXCLUDED.os,
			is_ipv6=EXCLUDED.is_ipv6, open_ports=EXCLUDED.open_ports, last_seen=EXCLUDED.last_seen`,
		h.IP, h.ASN, h.Org, geo, h.OS, h.IsIPv6, h.OpenPorts, h.FirstSeen, h.LastSeen)
	if err != nil {
		return err
	}
	if s.es != nil {
		doc := map[string]any{
			"doc_type":  "host",
			"ip":        h.IP,
			"asn":       h.ASN,
			"org":       h.Org,
			"os":        h.OS,
			"is_ipv6":   h.IsIPv6,
			"open_ports": h.OpenPorts,
			"geo":       h.Geo,
			"last_seen": h.LastSeen,
		}
		s.indexAsset(ctx, "host:"+h.IP, doc)
	}
	return nil
}

// UpsertPort 写入/更新端口资产（按 ip+port+proto 冲突更新），并同步 ES
func (s *Store) UpsertPort(ctx context.Context, p model.Port) error {
	cert, _ := json.Marshal(p.Cert)
	web, _ := json.Marshal(p.WebInfo)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ports (ip, port, proto, service, version, banner, cert, title, host, is_ipv6, webinfo, first_seen, last_seen)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (ip, port, proto) DO UPDATE SET
			service=EXCLUDED.service, version=EXCLUDED.version, banner=EXCLUDED.banner,
			cert=EXCLUDED.cert, title=EXCLUDED.title, host=EXCLUDED.host, is_ipv6=EXCLUDED.is_ipv6,
			webinfo=EXCLUDED.webinfo, last_seen=EXCLUDED.last_seen`,
		p.IP, p.Port, p.Proto, p.Service, p.Version, p.Banner, cert, p.Title, p.Host, p.IsIPv6, web, p.FirstSeen, p.LastSeen)
	if err != nil {
		return err
	}
	if s.es != nil {
		server, _ := p.WebInfo["server"].(string)
		tech, _ := p.WebInfo["tech"].([]string)
		doc := map[string]any{
			"doc_type": "port",
			"ip":       p.IP,
			"port":     p.Port,
			"proto":    p.Proto,
			"service":  p.Service,
			"version":  p.Version,
			"banner":   p.Banner,
			"title":    p.Title,
			"host":     p.Host,
			"is_ipv6":  p.IsIPv6,
			"server":   server,
			"tech":     tech,
			"last_seen": p.LastSeen,
		}
		s.indexAsset(ctx, "port:"+p.IP+":"+strconv.Itoa(p.Port), doc)
	}
	return nil
}

// UpsertDomain 写入/更新域名资产（按 name 冲突更新）
func (s *Store) UpsertDomain(ctx context.Context, d model.Domain) error {
	whois, _ := json.Marshal(d.Whois)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO domains (name, registrable_domain, resolved_ips, cname, org, asn, is_ipv6, whois, first_seen, last_seen)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (name) DO UPDATE SET
			registrable_domain=EXCLUDED.registrable_domain, resolved_ips=EXCLUDED.resolved_ips,
			cname=EXCLUDED.cname, org=EXCLUDED.org, asn=EXCLUDED.asn, is_ipv6=EXCLUDED.is_ipv6,
			whois=EXCLUDED.whois, last_seen=EXCLUDED.last_seen`,
		d.Name, d.RegistrableDomain, d.ResolvedIPs, d.CNAME, d.Org, d.ASN, d.IsIPv6, whois, d.FirstSeen, d.LastSeen)
	if err != nil {
		return err
	}
	if s.es != nil {
		doc := map[string]any{
			"doc_type":          "domain",
			"name":              d.Name,
			"host":              d.Name, // 复用 host 字段，使 host=/domain= 检索一致
			"registrable_domain": d.RegistrableDomain,
			"org":               d.Org,
			"asn":               d.ASN,
			"is_ipv6":           d.IsIPv6,
			"last_seen":         d.LastSeen,
		}
		s.indexAsset(ctx, "domain:"+d.Name, doc)
	}
	return nil
}

// ListDomains 列出域名资产
func (s *Store) ListDomains(ctx context.Context) ([]model.Domain, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT name, registrable_domain, resolved_ips, cname, org, asn, is_ipv6, whois, first_seen, last_seen
		FROM domains ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Domain{}
	for rows.Next() {
		var d model.Domain
		var resolved, cname []string
		var whois []byte
		if err := rows.Scan(&d.Name, &d.RegistrableDomain, &resolved, &cname, &d.Org, &d.ASN, &d.IsIPv6, &whois, &d.FirstSeen, &d.LastSeen); err != nil {
			return nil, err
		}
		d.ResolvedIPs = resolved
		d.CNAME = cname
		_ = json.Unmarshal(whois, &d.Whois)
		out = append(out, d)
	}
	return out, rows.Err()
}

// InsertAudit 写入审计记录（审计开关由调用方控制）
func (s *Store) InsertAudit(ctx context.Context, operator, target, taskID, action string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (operator, target, task_id, action) VALUES ($1,$2,$3,$4)`,
		operator, target, taskID, action)
	return err
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

// GetHost 按 IP 读取主机资产（含开放端口）
func (s *Store) GetHost(ctx context.Context, ip string) (model.Host, error) {
	var h model.Host
	var geo, openPorts []byte
	err := s.pool.QueryRow(ctx, `
		SELECT ip, asn, org, geo, os, is_ipv6, open_ports, first_seen, last_seen
		FROM hosts WHERE ip=$1`, ip).
		Scan(&h.IP, &h.ASN, &h.Org, &geo, &h.OS, &h.IsIPv6, &openPorts, &h.FirstSeen, &h.LastSeen)
	if err != nil {
		return h, err
	}
	_ = json.Unmarshal(geo, &h.Geo)
	_ = json.Unmarshal(openPorts, &h.OpenPorts)
	return h, nil
}

// ListPortsByIP 列出某 IP 的全部端口资产（含 host/is_ipv6/指纹）
func (s *Store) ListPortsByIP(ctx context.Context, ip string) ([]model.Port, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ip, port, proto, service, version, title, banner, host, is_ipv6, webinfo
		FROM ports WHERE ip=$1 ORDER BY port`, ip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Port{}
	for rows.Next() {
		var p model.Port
		var web []byte
		if err := rows.Scan(&p.IP, &p.Port, &p.Proto, &p.Service, &p.Version, &p.Title,
			&p.Banner, &p.Host, &p.IsIPv6, &web); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(web, &p.WebInfo)
		out = append(out, p)
	}
	return out, rows.Err()
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

// SearchAssets 见 query.go（表达式/Dork 检索，参考 FOFA 语法）

// CreateTask 持久化任务
func (s *Store) CreateTask(ctx context.Context, t model.Task) error {
	scope, _ := json.Marshal(t.Scope)
	sched, _ := json.Marshal(t.Schedule)
	rl, _ := json.Marshal(t.RateLimit)
	prog, _ := json.Marshal(t.Progress)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tasks (id, kind, scope, schedule, rate_limit, status, progress)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		t.ID, t.Kind, scope, sched, rl, t.Status, prog)
	return err
}

// GetTask 按 ID 读取任务
func (s *Store) GetTask(ctx context.Context, id string) (model.Task, error) {
	var t model.Task
	var scope, sched, rl, prog []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, kind, scope, schedule, rate_limit, status, progress, created_at
		FROM tasks WHERE id=$1`, id).
		Scan(&t.ID, &t.Kind, &scope, &sched, &rl, &t.Status, &prog, &t.CreatedAt)
	if err != nil {
		return t, err
	}
	_ = json.Unmarshal(scope, &t.Scope)
	_ = json.Unmarshal(sched, &t.Schedule)
	_ = json.Unmarshal(rl, &t.RateLimit)
	_ = json.Unmarshal(prog, &t.Progress)
	return t, nil
}

// ListTasks 列出全部任务（倒序）
func (s *Store) ListTasks(ctx context.Context) ([]model.Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, kind, scope, schedule, rate_limit, status, progress, created_at
		FROM tasks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Task{}
	for rows.Next() {
		var t model.Task
		var scope, sched, rl, prog []byte
		if err := rows.Scan(&t.ID, &t.Kind, &scope, &sched, &rl, &t.Status, &prog, &t.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(scope, &t.Scope)
		_ = json.Unmarshal(sched, &t.Schedule)
		_ = json.Unmarshal(rl, &t.RateLimit)
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
