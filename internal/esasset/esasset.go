package esasset

import (
	"context"
	"errors"

	"atlas/internal/assetstore"
	"atlas/internal/model"
	"atlas/internal/store"
)

// ESAssetStore 以 Elasticsearch 为资产唯一存储的实现
type ESAssetStore struct{ es *store.ESClient }

// New 构造 ES 资产存储
func New(es *store.ESClient) *ESAssetStore { return &ESAssetStore{es: es} }

// Upsert 写入/更新资产（统一 model.Asset；_id 由 model.AssetID 决定）
func (s *ESAssetStore) Upsert(ctx context.Context, a model.Asset) error {
	doc := assetToDoc(a)
	return s.es.IndexAsset(ctx, model.AssetID(a), doc)
}

// assetToDoc 将统一 Asset 转为 ES 文档；跳过零值以减小体积
func assetToDoc(a model.Asset) map[string]any {
	m := map[string]any{"doc_type": string(a.Kind)}
	if a.IP != "" {
		m["ip"] = a.IP
	}
	if a.Port != 0 {
		m["port"] = a.Port
	}
	if a.Proto != "" {
		m["proto"] = a.Proto
	}
	if a.Domain != "" {
		m["domain"] = a.Domain
	}
	if a.Host != "" {
		m["host"] = a.Host
	}
	if a.ASN != 0 {
		m["asn"] = a.ASN
	}
	if a.Org != "" {
		m["org"] = a.Org
	}
	if a.OS != "" {
		m["os"] = a.OS
	}
	if a.IsIPv6 {
		m["is_ipv6"] = true
	}
	if a.State != "" {
		m["state"] = a.State
	}
	if a.Service != "" {
		m["service"] = a.Service
	}
	if a.Version != "" {
		m["version"] = a.Version
	}
	if a.Banner != "" {
		m["banner"] = a.Banner
	}
	if a.Title != "" {
		m["title"] = a.Title
	}
	if a.Server != "" {
		m["server"] = a.Server
	}
	if len(a.Tech) > 0 {
		m["tech"] = a.Tech
	}
	if a.RegistrableDomain != "" {
		m["registrable_domain"] = a.RegistrableDomain
	}
	if len(a.ResolvedIPs) > 0 {
		m["resolved_ips"] = a.ResolvedIPs
	}
	if len(a.CNAME) > 0 {
		m["cname"] = a.CNAME
	}
	if a.OpenPorts != 0 {
		m["open_ports"] = a.OpenPorts
	}
	if len(a.Cert) > 0 {
		m["cert"] = a.Cert
	}
	if len(a.WebInfo) > 0 {
		m["webinfo"] = a.WebInfo
	}
	if len(a.Geo) > 0 {
		m["geo"] = a.Geo
	}
	if len(a.Whois) > 0 {
		m["whois"] = a.Whois
	}
	if !a.FirstSeen.IsZero() {
		m["first_seen"] = a.FirstSeen
	}
	if !a.LastSeen.IsZero() {
		m["last_seen"] = a.LastSeen
	}
	return m
}

// GetHost 按 IP 读取主机资产；未找到返回 assetstore.ErrNotFound
func (s *ESAssetStore) GetHost(ctx context.Context, ip string) (model.Asset, error) {
	src, err := s.es.Get(ctx, "host:"+ip)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Asset{}, assetstore.ErrNotFound
		}
		return model.Asset{}, err
	}
	return assetFromSource(src), nil
}

// ListPortsByIP 列出某 IP 的全部端口（按 port 升序）
func (s *ESAssetStore) ListPortsByIP(ctx context.Context, ip string) ([]model.Asset, error) {
	q := map[string]any{
		"query": map[string]any{"bool": map[string]any{"must": []any{
			map[string]any{"term": map[string]any{"doc_type": "port"}},
			map[string]any{"term": map[string]any{"ip": ip}},
		}}},
		"sort":  []any{map[string]any{"port": map[string]any{"order": "asc"}}},
		"size":  10000,
	}
	items, _, err := s.es.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]model.Asset, 0, len(items))
	for _, it := range items {
		out = append(out, assetFromSource(it))
	}
	return out, nil
}

// ListDomains 列出全部域名（按 last_seen 倒序）
func (s *ESAssetStore) ListDomains(ctx context.Context) ([]model.Asset, error) {
	q := map[string]any{
		"query": map[string]any{"term": map[string]any{"doc_type": "domain"}},
		"sort":  []any{map[string]any{"last_seen": map[string]any{"order": "desc"}}},
		"size":  10000,
	}
	items, _, err := s.es.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]model.Asset, 0, len(items))
	for _, it := range items {
		out = append(out, assetFromSource(it))
	}
	return out, nil
}

// GetHostDetail 主机 + 全部端口（漏洞由调用方从 PG 取）
func (s *ESAssetStore) GetHostDetail(ctx context.Context, ip string) (model.Asset, []model.Asset, error) {
	h, err := s.GetHost(ctx, ip)
	if err != nil {
		return model.Asset{}, nil, err
	}
	ports, err := s.ListPortsByIP(ctx, ip)
	if err != nil {
		return h, nil, err
	}
	return h, ports, nil
}

// assetFromSource 从 ES _source 还原统一 Asset
func assetFromSource(m map[string]any) model.Asset {
	a := model.Asset{}
	if v, ok := m["doc_type"].(string); ok {
		a.Kind = model.AssetKind(v)
	}
	if v, ok := m["ip"].(string); ok {
		a.IP = v
	}
	if v, ok := m["port"].(float64); ok {
		a.Port = int(v)
	}
	if v, ok := m["proto"].(string); ok {
		a.Proto = v
	}
	if v, ok := m["domain"].(string); ok {
		a.Domain = v
	}
	if v, ok := m["host"].(string); ok {
		a.Host = v
	}
	if v, ok := m["asn"].(float64); ok {
		a.ASN = int(v)
	}
	if v, ok := m["org"].(string); ok {
		a.Org = v
	}
	if v, ok := m["os"].(string); ok {
		a.OS = v
	}
	if v, ok := m["is_ipv6"].(bool); ok {
		a.IsIPv6 = v
	}
	if v, ok := m["state"].(string); ok {
		a.State = v
	}
	if v, ok := m["service"].(string); ok {
		a.Service = v
	}
	if v, ok := m["version"].(string); ok {
		a.Version = v
	}
	if v, ok := m["banner"].(string); ok {
		a.Banner = v
	}
	if v, ok := m["title"].(string); ok {
		a.Title = v
	}
	if v, ok := m["server"].(string); ok {
		a.Server = v
	}
	if v, ok := m["tech"].([]any); ok {
		ts := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				ts = append(ts, s)
			}
		}
		a.Tech = ts
	}
	if v, ok := m["registrable_domain"].(string); ok {
		a.RegistrableDomain = v
	}
	if v, ok := m["resolved_ips"].([]any); ok {
		rs := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				rs = append(rs, s)
			}
		}
		a.ResolvedIPs = rs
	}
	if v, ok := m["cname"].([]any); ok {
		cs := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				cs = append(cs, s)
			}
		}
		a.CNAME = cs
	}
	if v, ok := m["open_ports"].(float64); ok {
		a.OpenPorts = int(v)
	}
	if v, ok := m["cert"].(map[string]any); ok {
		a.Cert = v
	}
	if v, ok := m["webinfo"].(map[string]any); ok {
		a.WebInfo = v
	}
	if v, ok := m["geo"].(map[string]any); ok {
		a.Geo = v
	}
	if v, ok := m["whois"].(map[string]any); ok {
		a.Whois = v
	}
	return a
}
