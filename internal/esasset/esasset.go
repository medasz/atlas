package esasset

import (
	"context"

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
