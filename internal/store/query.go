package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

// ---- 查询表达式抽象语法树（参考 FOFA Dork 语法）----

type node interface {
	toES() map[string]any
	toPG(scope string, acc *[]any) string
}

type cmpNode struct {
	field, op, val string // op: = == != *=
}

type andNode struct{ l, r node }
type orNode struct{ l, r node }

// ---- 字段解析 ----

type fdef struct {
	es        string // ES 字段名（port/host 文档；domain 文档复用 host 字段）
	esKind    string // ip|int|keyword|text|bool|all
	all       bool   // 是否为全字段模糊
	hostCol   string // hosts 表对应列（空表示不适用）
	portCol   string // ports 表对应列（空表示不适用）
	domainCol string // domains 表对应列（空表示不适用）
	pgKind    string // int|text|bool（用于 PG 比较方式）
}

var fieldDefs = map[string]fdef{
	"ip":            {"ip", "ip", false, "ip::text", "ip::text", "ip::text", "text"},
	"port":          {"port", "int", false, "", "port", "", "int"},
	"protocol":      {"proto", "keyword", false, "", "proto", "", "text"},
	"base_protocol": {"proto", "keyword", false, "", "proto", "", "text"},
	"server":        {"server", "keyword", false, "", "webinfo->>'server'", "", "text"},
	"banner":        {"banner", "text", false, "", "banner", "", "text"},
	"title":         {"title", "text", false, "", "title", "", "text"},
	"os":            {"os", "keyword", false, "os", "", "", "text"},
	"org":           {"org", "keyword", false, "org", "", "", "text"},
	"asn":           {"asn", "int", false, "asn", "", "", "int"},
	"host":          {"host", "keyword", false, "", "host", "name", "text"},
	"domain":        {"host", "keyword", false, "", "host", "name", "text"},
	"app":           {"tech", "keyword", false, "", "webinfo->>'tech'", "", "text"},
	"product":       {"tech", "keyword", false, "", "webinfo->>'tech'", "", "text"},
	"body":          {"_all", "all", true, "", "", "", ""},
	"header":        {"_all", "all", true, "", "", "", ""},
	"country":       {"geo.country", "keyword", false, "geo->>'country'", "", "", "text"},
	"region":        {"geo.region", "keyword", false, "geo->>'region'", "", "", "text"},
	"city":          {"geo.city", "keyword", false, "geo->>'city'", "", "", "text"},
	"cert":          {"_all", "all", true, "", "cert::text", "", ""},
	"is_ipv6":       {"is_ipv6", "bool", false, "is_ipv6", "is_ipv6", "is_ipv6", "bool"},
	"_all":          {"_all", "all", true, "", "", "", ""},
}

// scope 内用于全字段模糊的列
var scopeAllCols = map[string][]string{
	"host":   {"ip::text", "org", "os"},
	"port":   {"ip::text", "host", "service", "version", "title", "banner", "webinfo->>'server'", "webinfo->>'tech'", "cert::text"},
	"domain": {"name", "registrable_domain", "org"},
}

var esAllFields = []string{"ip", "host", "service", "version", "title", "banner", "server", "tech", "org", "os"}

// ---- 词法 / 语法分析 ----

type tokType int

const (
	tLParen tokType = iota
	tRParen
	tAnd
	tOr
	tCmp
	tBare
)

type tok struct {
	t              tokType
	field, op, val string
}

func tokenize(q string) ([]tok, error) {
	runes := []rune(q)
	n := len(runes)
	var out []tok
	i := 0
	for i < n {
		c := runes[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if c == '(' {
			out = append(out, tok{t: tLParen})
			i++
			continue
		}
		if c == ')' {
			out = append(out, tok{t: tRParen})
			i++
			continue
		}
		if i+1 < n && runes[i] == '&' && runes[i+1] == '&' {
			out = append(out, tok{t: tAnd})
			i += 2
			continue
		}
		if i+1 < n && runes[i] == '|' && runes[i+1] == '|' {
			out = append(out, tok{t: tOr})
			i += 2
			continue
		}
		// 读取一个 term（值含引号时允许空格）
		j := i
		for j < n {
			ch := runes[j]
			if ch == '"' {
				j++
				for j < n && runes[j] != '"' {
					j++
				}
				if j < n {
					j++
				}
				continue
			}
			if ch == '(' || ch == ')' || ch == '&' || ch == '|' || ch == ' ' {
				break
			}
			j++
		}
		term := string(runes[i:j])
		i = j
		if f, o, v, ok := splitCmp(term); ok {
			out = append(out, tok{t: tCmp, field: f, op: o, val: v})
		} else {
			out = append(out, tok{t: tBare, val: term})
		}
	}
	return out, nil
}

// splitCmp 在引号外寻找首个运算符，拆分 field/op/val
func splitCmp(term string) (field, op, val string, ok bool) {
	inQ := false
	for i := 0; i < len(term); i++ {
		c := term[i]
		if c == '"' {
			inQ = !inQ
			continue
		}
		if inQ {
			continue
		}
		if i+1 < len(term) {
			two := term[i : i+2]
			if two == "==" || two == "!=" || two == "*=" {
				return term[:i], two, unquote(term[i+2:]), true
			}
		}
		if c == '=' {
			return term[:i], "=", unquote(term[i+1:]), true
		}
	}
	return "", "", "", false
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// parseQuery 解析为 AST；无有效节点返回 nil（视为无过滤）
func parseQuery(q string) node {
	toks, err := tokenize(q)
	if err != nil || len(toks) == 0 {
		return nil
	}
	p := &parser{toks: toks}
	n := p.parseOr()
	if n == nil {
		return nil
	}
	return n
}

type parser struct {
	toks []tok
	pos  int
}

func (p *parser) peek() tok {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return tok{t: -1}
}

func (p *parser) next() tok {
	t := p.peek()
	p.pos++
	return t
}

func (p *parser) parseOr() node {
	left := p.parseAnd()
	for p.peek().t == tOr {
		p.next()
		right := p.parseAnd()
		left = orNode{left, right}
	}
	return left
}

func (p *parser) parseAnd() node {
	left := p.parseTerm()
	for p.peek().t == tAnd {
		p.next()
		right := p.parseTerm()
		left = andNode{left, right}
	}
	return left
}

func (p *parser) parseTerm() node {
	t := p.peek()
	if t.t == tLParen {
		p.next()
		n := p.parseOr()
		if p.peek().t == tRParen {
			p.next()
		}
		return n
	}
	return p.parseCmp()
}

func (p *parser) parseCmp() node {
	t := p.next()
	switch t.t {
	case tCmp:
		return cmpNode{field: strings.ToLower(t.field), op: t.op, val: t.val}
	case tBare:
		return cmpNode{field: "_all", op: "=", val: t.val}
	default:
		return nil
	}
}

// ---- ES 渲染 ----

func (c cmpNode) toES() map[string]any {
	d, ok := fieldDefs[c.field]
	if !ok {
		d = fieldDefs["_all"]
	}
	var clause map[string]any
	switch {
	case d.all:
		clause = map[string]any{"multi_match": map[string]any{"query": c.val, "fields": esAllFields}}
	case d.esKind == "ip":
		if c.op == "*=" {
			clause = map[string]any{"wildcard": map[string]any{d.es: wildES(c.val)}}
		} else {
			clause = map[string]any{"term": map[string]any{d.es: c.val}}
		}
	case d.esKind == "int":
		if v, err := strconv.Atoi(c.val); err == nil {
			clause = map[string]any{"term": map[string]any{d.es: v}}
		} else {
			clause = map[string]any{"match": map[string]any{d.es: c.val}}
		}
	case d.esKind == "bool":
		clause = map[string]any{"term": map[string]any{d.es: parseBool(c.val)}}
	default: // keyword / text
		switch c.op {
		case "==":
			clause = map[string]any{"term": map[string]any{d.es: c.val}}
		case "*=":
			clause = map[string]any{"wildcard": map[string]any{d.es: wildES(c.val)}}
		default: // "=", contains
			clause = map[string]any{"match": map[string]any{d.es: c.val}}
		}
	}
	if c.op == "!=" {
		return map[string]any{"bool": map[string]any{"must_not": clause}}
	}
	return clause
}

// parseBool 将 Dork 值解析为布尔（true/1/yes → true；其余 → false）
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "y":
		return true
	default:
		return false
	}
}

func (a andNode) toES() map[string]any {
	return map[string]any{"bool": map[string]any{"must": []any{a.l.toES(), a.r.toES()}}}
}

func (o orNode) toES() map[string]any {
	return map[string]any{"bool": map[string]any{"should": []any{o.l.toES(), o.r.toES()}, "minimum_should_match": 1}}
}

func wildES(v string) string {
	if !strings.ContainsAny(v, "*?") {
		return "*" + v + "*"
	}
	return v
}

// ---- PG 渲染 ----

func renumber(sql string, args []any, acc *[]any) string {
	out := sql
	for _, a := range args {
		*acc = append(*acc, a)
		out = strings.Replace(out, "?", "$"+strconv.Itoa(len(*acc)), 1)
	}
	return out
}

func likeExpr(col, kind string, c cmpNode) (string, []any) {
	if kind == "bool" {
		bv := parseBool(c.val)
		if c.op == "!=" {
			return col + " <> ?", []any{bv}
		}
		return col + " = ?", []any{bv}
	}
	if kind == "int" {
		if v, err := strconv.Atoi(c.val); err == nil {
			if c.op == "!=" {
				return col + " <> ?", []any{v}
			}
			return col + " = ?", []any{v}
		}
		return "TRUE", nil
	}
	if c.op == "!=" {
		return col + " NOT ILIKE ?", []any{"%" + c.val + "%"}
	}
	if c.op == "*=" {
		return col + " ILIKE ?", []any{wildPG(c.val)}
	}
	return col + " ILIKE ?", []any{"%" + c.val + "%"}
}

func (c cmpNode) toPG(scope string, acc *[]any) string {
	d, ok := fieldDefs[c.field]
	if !ok {
		d = fieldDefs["_all"]
	}
	if d.all {
		cols := scopeAllCols[scope]
		var sqls []string
		var args []any
		for _, col := range cols {
			s, a := likeExpr(col, "text", c)
			sqls = append(sqls, s)
			args = append(args, a...)
		}
		return renumber("("+strings.Join(sqls, " OR ")+")", args, acc)
	}
	col, kind, ok := pgCol(d, scope)
	if !ok {
		return "TRUE"
	}
	s, a := likeExpr(col, kind, c)
	return renumber(s, a, acc)
}

func (a andNode) toPG(scope string, acc *[]any) string {
	return "(" + a.l.toPG(scope, acc) + " AND " + a.r.toPG(scope, acc) + ")"
}

func (o orNode) toPG(scope string, acc *[]any) string {
	return "(" + o.l.toPG(scope, acc) + " OR " + o.r.toPG(scope, acc) + ")"
}

func pgCol(d fdef, scope string) (string, string, bool) {
	switch scope {
	case "host":
		if d.hostCol == "" {
			return "", "", false
		}
		return d.hostCol, d.pgKind, true
	case "domain":
		if d.domainCol == "" {
			return "", "", false
		}
		return d.domainCol, d.pgKind, true
	default: // port
		if d.portCol == "" {
			return "", "", false
		}
		return d.portCol, d.pgKind, true
	}
}

func wildPG(v string) string {
	v = strings.ReplaceAll(v, "*", "%")
	v = strings.ReplaceAll(v, "?", "_")
	if !strings.ContainsAny(v, "%_") {
		return "%" + v + "%"
	}
	return v
}

// ---- 检索入口（含标准分页） ----

// SearchResult 资产检索的结构化分页结果
type SearchResult struct {
	Total      int64            `json:"total"`       // 总记录数
	Page       int              `json:"page"`        // 当前页码（从 1 开始）
	PageSize   int              `json:"page_size"`   // 每页条数
	TotalPages int              `json:"total_pages"` // 总页数
	Items      []map[string]any `json:"items"`       // 当前页数据列表
}

// assetCols host/port/domain 三表 UNION 的公共投影列（缺列补 NULL）
const assetCols = `doc_type, ip, port, proto, name, org, os, asn, is_ipv6, banner, title, service, version, registrable_domain, server`

// SearchAssets 资产检索：键式/表达式语法（参考 FOFA Dork），优先 ES，未配置则回退 PG。
// page 从 1 开始，pageSize 为每页条数，返回带总数与总页数的标准分页信封。
func (s *Store) SearchAssets(ctx context.Context, q, docType string, page, pageSize int) (*SearchResult, error) {
	root := parseQuery(q)
	if s.es != nil {
		esQuery := buildESQuery(root, docType, (page-1)*pageSize, pageSize)
		if items, total, err := s.es.Search(ctx, esQuery); err == nil {
			tp := pageSize
			if tp <= 0 {
				tp = 20
			}
			totalPages := 0
			if tp > 0 && total > 0 {
				totalPages = int((total + int64(tp) - 1) / int64(tp))
			}
			return &SearchResult{Total: total, Page: page, PageSize: tp, TotalPages: totalPages, Items: items}, nil
		}
	}
	return s.searchAssetsPG(ctx, root, docType, page, pageSize)
}

// buildESQuery 生成 ES 查询并注入分页（from/size）
func buildESQuery(root node, docType string, from, size int) map[string]any {
	must := []any{}
	if docType != "" {
		must = append(must, map[string]any{"term": map[string]any{"doc_type": docType}})
	}
	if root != nil {
		must = append(must, root.toES())
	}
	if size <= 0 {
		size = 20
	}
	if from < 0 {
		from = 0
	}
	if len(must) == 0 {
		return map[string]any{"query": map[string]any{"match_all": map[string]any{}}, "from": from, "size": size}
	}
	return map[string]any{"query": map[string]any{"bool": map[string]any{"must": must}}, "from": from, "size": size}
}

// scopeUnionSelect 生成单个 scope 的 SELECT，投影到 assetCols（缺列补 NULL）
func scopeUnionSelect(scope, where string) string {
	switch scope {
	case "host":
		return `SELECT 'host' doc_type, ip, NULL::int AS port, NULL::text AS proto, NULL::text AS name, org, os, asn, is_ipv6, NULL::text AS banner, NULL::text AS title, NULL::text AS service, NULL::text AS version, NULL::text AS registrable_domain, NULL::text AS server FROM hosts WHERE ` + where
	case "port":
		return `SELECT 'port' doc_type, ip, port, proto, host AS name, NULL::text AS org, NULL::text AS os, NULL::int AS asn, NULL::bool AS is_ipv6, banner, title, service, version, NULL::text AS registrable_domain, webinfo->>'server' AS server FROM ports WHERE ` + where
	case "domain":
		return `SELECT 'domain' doc_type, NULL::text AS ip, NULL::int AS port, NULL::text AS proto, name, NULL::text AS org, NULL::text AS os, NULL::int AS asn, NULL::bool AS is_ipv6, NULL::text AS banner, NULL::text AS title, NULL::text AS service, NULL::text AS version, registrable_domain, NULL::text AS server FROM domains WHERE ` + where
	}
	// 兜底（理论上不会命中）：返回空结果
	return `SELECT 'host' doc_type, ip, NULL::int AS port, NULL::text AS proto, NULL::text AS name, org, os, asn, is_ipv6, NULL::text AS banner, NULL::text AS title, NULL::text AS service, NULL::text AS version, NULL::text AS registrable_domain, NULL::text AS server FROM hosts WHERE FALSE`
}

// searchAssetsPG PG 回退：以 host/port/domain 三表 UNION 实现统一、天然去重、分页检索。
// 指定 docType 时退化为单表查询，性能最佳。各表唯一约束保证 UNION 行天然唯一（无同 IP 多行重复）。
func (s *Store) searchAssetsPG(ctx context.Context, root node, docType string, page, pageSize int) (*SearchResult, error) {
	scopes := []string{"host", "port", "domain"}
	if docType != "" {
		scopes = []string{docType}
	}

	var wArgs []any
	branches := make([]string, 0, len(scopes))
	for _, sc := range scopes {
		where := "TRUE"
		if root != nil {
			where = root.toPG(sc, &wArgs)
		}
		branches = append(branches, scopeUnionSelect(sc, where))
	}
	union := strings.Join(branches, " UNION ALL ")

	// 总数：外层 count(*) 覆盖 UNION 全部命中行
	var acc []any
	unionNumbered := renumber(union, wArgs, &acc)
	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM (`+unionNumbered+`) sub`, acc...).Scan(&total); err != nil {
		return nil, err
	}

	// 分页数据：按 ip/port 稳定排序，domain 的 ip 为 NULL 排末尾
	offset := (page - 1) * pageSize
	dataSQL := `SELECT ` + assetCols + ` FROM (` + unionNumbered + `) sub ORDER BY ip NULLS LAST, port NULLS LAST LIMIT ? OFFSET ?`
	dSQL := renumber(dataSQL, []any{pageSize, offset}, &acc)
	rows, err := s.pool.Query(ctx, dSQL, acc...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0, pageSize)
	for rows.Next() {
		var (
			docType string
			ip      sql.NullString
			port    sql.NullInt32
			proto   sql.NullString
			name    sql.NullString
			org     sql.NullString
			os      sql.NullString
			asn     sql.NullInt32
			isIPv6  sql.NullBool
			banner  sql.NullString
			title   sql.NullString
			service sql.NullString
			version sql.NullString
			reg     sql.NullString
			server  sql.NullString
		)
		if err := rows.Scan(&docType, &ip, &port, &proto, &name, &org, &os, &asn, &isIPv6,
			&banner, &title, &service, &version, &reg, &server); err != nil {
			return nil, err
		}
		m := map[string]any{"doc_type": docType}
		if ip.Valid {
			m["ip"] = ip.String
		}
		if port.Valid {
			m["port"] = int(port.Int32)
		}
		if proto.Valid {
			m["proto"] = proto.String
		}
		if name.Valid {
			m["name"] = name.String
		}
		if org.Valid {
			m["org"] = org.String
		}
		if os.Valid {
			m["os"] = os.String
		}
		if asn.Valid {
			m["asn"] = int(asn.Int32)
		}
		m["is_ipv6"] = isIPv6.Valid && isIPv6.Bool
		if banner.Valid {
			m["banner"] = banner.String
		}
		if title.Valid {
			m["title"] = title.String
		}
		if service.Valid {
			m["service"] = service.String
		}
		if version.Valid {
			m["version"] = version.String
		}
		if reg.Valid {
			m["registrable_domain"] = reg.String
		}
		if server.Valid {
			m["server"] = server.String
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	totalPages := 0
	if pageSize > 0 && total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &SearchResult{Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages, Items: out}, nil
}
