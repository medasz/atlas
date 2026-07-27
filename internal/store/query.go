package store

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"atlas/internal/model"
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
type orNode  struct{ l, r node }

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
	t          tokType
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

// ---- 检索入口 ----

// SearchAssets 资产检索：键式/表达式语法（参考 FOFA Dork），优先 ES，未配置则回退 PG
func (s *Store) SearchAssets(ctx context.Context, q string, docType string) ([]map[string]any, error) {
	root := parseQuery(q)
	if s.es != nil {
		esQuery := buildESQuery(root, docType)
		if items, err := s.es.Search(ctx, esQuery); err == nil {
			return items, nil
		}
	}
	return s.searchAssetsPG(ctx, root, docType)
}

func buildESQuery(root node, docType string) map[string]any {
	must := []any{}
	if docType != "" {
		must = append(must, map[string]any{"term": map[string]any{"doc_type": docType}})
	}
	if root != nil {
		must = append(must, root.toES())
	}
	if len(must) == 0 {
		return map[string]any{"query": map[string]any{"match_all": map[string]any{}}, "size": 100}
	}
	return map[string]any{"query": map[string]any{"bool": map[string]any{"must": must}}, "size": 100}
}

func (s *Store) searchAssetsPG(ctx context.Context, root node, docType string) ([]map[string]any, error) {
	out := []map[string]any{}
	if docType == "" || docType == "host" {
		rows, err := s.queryHostsPG(ctx, root)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var h model.Host
			var openPorts []byte
			if err := rows.Scan(&h.IP, &h.ASN, &h.Org, &h.OS, &openPorts); err != nil {
				rows.Close()
				return nil, err
			}
			_ = json.Unmarshal(openPorts, &h.OpenPorts)
			out = append(out, map[string]any{"doc_type": "host", "ip": h.IP, "org": h.Org, "os": h.OS, "open_ports": h.OpenPorts})
		}
		rows.Close()
	}
	if docType == "" || docType == "port" {
		rows, err := s.queryPortsPG(ctx, root)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p model.Port
			if err := rows.Scan(&p.IP, &p.Port, &p.Proto, &p.Service, &p.Version, &p.Title, &p.Banner); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, map[string]any{"doc_type": "port", "ip": p.IP, "port": p.Port, "proto": p.Proto,
				"service": p.Service, "version": p.Version, "title": p.Title, "banner": p.Banner})
		}
		rows.Close()
	}
	if docType == "" || docType == "domain" {
		rows, err := s.queryDomainsPG(ctx, root)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var d model.Domain
			if err := rows.Scan(&d.Name, &d.RegistrableDomain, &d.Org, &d.ASN, &d.IsIPv6); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, map[string]any{"doc_type": "domain", "name": d.Name,
				"registrable_domain": d.RegistrableDomain, "org": d.Org, "asn": d.ASN, "is_ipv6": d.IsIPv6})
		}
		rows.Close()
	}
	return out, nil
}

func (s *Store) queryDomainsPG(ctx context.Context, root node) (pgx.Rows, error) {
	var acc []any
	where := "TRUE"
	if root != nil {
		where = root.toPG("domain", &acc)
	}
	return s.pool.Query(ctx, `SELECT name, registrable_domain, org, asn, is_ipv6 FROM domains WHERE `+where, acc...)
}

func (s *Store) queryHostsPG(ctx context.Context, root node) (pgx.Rows, error) {
	var acc []any
	where := "TRUE"
	if root != nil {
		where = root.toPG("host", &acc)
	}
	return s.pool.Query(ctx, `SELECT ip, asn, org, os, open_ports FROM hosts WHERE `+where, acc...)
}

func (s *Store) queryPortsPG(ctx context.Context, root node) (pgx.Rows, error) {
	var acc []any
	where := "TRUE"
	if root != nil {
		where = root.toPG("port", &acc)
	}
	return s.pool.Query(ctx, `SELECT ip, port, proto, service, version, title, banner FROM ports WHERE `+where, acc...)
}
