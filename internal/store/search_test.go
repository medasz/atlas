package store

import (
	"testing"
)

func TestParseQueryFieldOp(t *testing.T) {
	c, ok := ParseQuery(`ip="1.1.1.1"`).(cmpNode)
	if !ok || c.field != "ip" || c.op != "=" || c.val != "1.1.1.1" {
		t.Fatalf("unexpected cmp: %#v", c)
	}
	es := c.toES()
	term, ok := es["term"].(map[string]any)
	if !ok || term["ip"] != "1.1.1.1" {
		t.Fatalf("expected term ip, got %#v", es)
	}
}

func TestParseQueryAndOr(t *testing.T) {
	root := ParseQuery(`ip="1.1.1.1" && port="443"`)
	if _, ok := root.(andNode); !ok {
		t.Fatalf("expected andNode, got %T", root)
	}
	es := root.toES()
	must, ok := es["bool"].(map[string]any)["must"].([]any)
	if !ok || len(must) != 2 {
		t.Fatalf("expected bool.must len 2, got %#v", es)
	}

	root = ParseQuery(`title="a" || server="b"`)
	if _, ok := root.(orNode); !ok {
		t.Fatalf("expected orNode, got %T", root)
	}
	es = root.toES()
	sh, ok := es["bool"].(map[string]any)["should"].([]any)
	if !ok || len(sh) != 2 {
		t.Fatalf("expected bool.should len 2, got %#v", es)
	}
}

func TestParseQueryNotAndWildcard(t *testing.T) {
	c := ParseQuery(`ip!="1.1.1.1"`).(cmpNode)
	if c.op != "!=" {
		t.Fatalf("expected !=, got %s", c.op)
	}
	es := c.toES()
	if _, ok := es["bool"].(map[string]any)["must_not"]; !ok {
		t.Fatalf("expected must_not, got %#v", es)
	}

	w := ParseQuery(`ip*="1.1.1.*"`).(cmpNode)
	wes := w.toES()
	if _, ok := wes["wildcard"]; !ok {
		t.Fatalf("expected wildcard, got %#v", wes)
	}
}

func TestParseQueryParens(t *testing.T) {
	root := ParseQuery(`(ip="1.1.1.1" || ip="2.2.2.2") && port="443"`)
	and, ok := root.(andNode)
	if !ok {
		t.Fatalf("expected andNode, got %T", root)
	}
	if _, ok := and.l.(orNode); !ok {
		t.Fatalf("expected left orNode, got %T", and.l)
	}
}

func TestParseQueryBareAndPG(t *testing.T) {
	c := ParseQuery(`nginx`).(cmpNode)
	if c.field != "_all" || c.val != "nginx" {
		t.Fatalf("expected _all nginx, got %#v", c)
	}
	var acc []any
	sql := c.toPG("port", &acc)
	if sql == "" || len(acc) != 9 {
		t.Fatalf("expected pg sql with 9 args, got %q %v", sql, acc)
	}
	// 端口字段在 port 作用域应映射到具体列（数值精确匹配）
	{
		var a []any
		p := ParseQuery(`port="443"`).(cmpNode)
		if sql := p.toPG("port", &a); sql != "port = $1" {
			t.Fatalf("expected port = $1, got %q", sql)
		}
		// 端口字段在 host 作用域不适用，应返回 TRUE
		if sql := p.toPG("host", &a); sql != "TRUE" {
			t.Fatalf("expected TRUE for port on host scope, got %q", sql)
		}
	}
}

func TestParseQueryHostDomainIPv6(t *testing.T) {
	// host 字段在 port 作用域映射到 host 文本列（文本用 ILIKE 包含匹配）
	{
		var acc []any
		h := ParseQuery(`host="example.com"`).(cmpNode)
		if sql := h.toPG("port", &acc); sql != "host ILIKE $1" {
			t.Fatalf("expected host ILIKE $1, got %q", sql)
		}
		// host 字段在 host 作用域不适用
		if sql := h.toPG("host", &acc); sql != "TRUE" {
			t.Fatalf("expected TRUE for host on host scope, got %q", sql)
		}
	}
	// domain 字段在 port 作用域映射到 host 列
	{
		var acc []any
		d := ParseQuery(`domain="example.com"`).(cmpNode)
		if sql := d.toPG("port", &acc); sql != "host ILIKE $1" {
			t.Fatalf("expected host ILIKE $1 for domain, got %q", sql)
		}
		// domain 字段在 domain 作用域映射到 name 列
		if sql := d.toPG("domain", &acc); sql == "" || sql == "TRUE" {
			t.Fatalf("expected domain scope sql, got %q", sql)
		}
	}
	// is_ipv6 布尔字段
	{
		var acc []any
		v := ParseQuery(`is_ipv6=true`).(cmpNode)
		if sql := v.toPG("port", &acc); sql != "is_ipv6 = $1" {
			t.Fatalf("expected is_ipv6 = $1, got %q", sql)
		}
		ves := v.toES()
		term, ok := ves["term"].(map[string]any)
		if !ok || term["is_ipv6"] != true {
			t.Fatalf("expected term is_ipv6 true, got %#v", ves)
		}
		// is_ipv6 != 渲染为 must_not
		vn := ParseQuery(`is_ipv6!=false`).(cmpNode)
		if _, ok := vn.toES()["bool"]; !ok {
			t.Fatalf("expected bool.must_not for is_ipv6!=, got %#v", vn.toES())
		}
	}
	// protocol/base_protocol 映射到 proto 列
	{
		var acc []any
		pr := ParseQuery(`protocol="tcp"`).(cmpNode)
		if sql := pr.toPG("port", &acc); sql != "proto ILIKE $1" {
			t.Fatalf("expected proto ILIKE $1, got %q", sql)
		}
	}
}
