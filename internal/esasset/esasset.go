package esasset

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"atlas/internal/assetstore"
	"atlas/internal/model"
	"atlas/internal/store"
)

// ESAssetStore 以 Elasticsearch 为资产唯一存储的实现
type ESAssetStore struct{ es *store.ESClient }

// New 构造 ES 资产存储
func New(es *store.ESClient) *ESAssetStore { return &ESAssetStore{es: es} }

// Upsert 写入/更新资产（使用 ES _update + doc_as_upsert + painless 脚本实现字段级合并语义）。
// first_seen 由服务端脚本保证仅在首次创建时设置，后续更新不会覆盖。
// last_seen 每次随 doc 写入更新。
func (s *ESAssetStore) Upsert(ctx context.Context, a model.Asset) error {
	doc := assetToDoc(a)
	return s.es.UpdateAsset(ctx, model.AssetID(a), doc)
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
		"sort": []any{map[string]any{"port": map[string]any{"order": "asc"}}},
		"size": 10000,
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

// SearchAssets 资产检索（仅 ES），支持标准分页及 Composite Aggregation (after_key 循环全量遍历) 聚合模式
func (s *ESAssetStore) SearchAssets(ctx context.Context, q, kind string, aggregated bool, page, pageSize int) (*store.SearchResult, error) {
	root := store.ParseQuery(q)
	tp := pageSize
	if tp <= 0 {
		tp = 20
	}
	if page < 1 {
		page = 1
	}

	if !aggregated {
		from := (page - 1) * tp
		if from < 0 {
			from = 0
		}
		query := store.BuildESQuery(root, kind, from, tp)
		items, total, err := s.es.Search(ctx, query)
		if err != nil {
			return nil, err
		}
		totalPages := 0
		if tp > 0 && total > 0 {
			totalPages = int((total + int64(tp) - 1) / int64(tp))
		}
		return &store.SearchResult{Total: total, Page: page, PageSize: tp, TotalPages: totalPages, Aggregated: false, Items: items}, nil
	}

	// ---- ES Composite Aggregation (after_key 循环遍历) 模式 ----
	aggregatedItems := make([]map[string]any, 0)

	skipIPAgg := (kind == "domain")
	skipDomainAgg := (kind == "host" || kind == "port")

	// 1. 循环遍历 IP 桶
	if !skipIPAgg {
		var ipAfterKey map[string]any
		for {
			compQuery := store.BuildESCompositeQuery(root, kind, false, ipAfterKey, 1000)
			rawResp, err := s.es.SearchAgg(ctx, compQuery)
			if err != nil {
				return nil, err
			}
			aggs, _ := rawResp["aggregations"].(map[string]any)
			if aggs == nil {
				break
			}
			compObj, _ := aggs["ip_composite"].(map[string]any)
			if compObj == nil {
				break
			}

			buckets, _ := compObj["buckets"].([]any)
			for _, b := range buckets {
				bMap, ok := b.(map[string]any)
				if !ok {
					continue
				}
				keyMap, _ := bMap["key"].(map[string]any)
				ipStr, _ := keyMap["ip"].(string)
				if ipStr == "" {
					ipStr, _ = bMap["key"].(string)
				}
				if ipStr == "" {
					continue
				}

				docCount, _ := bMap["doc_count"].(float64)

				topDocs, _ := bMap["top_docs"].(map[string]any)
				hitsObj, _ := topDocs["hits"].(map[string]any)
				hitsList, _ := hitsObj["hits"].([]any)

				isTruncated := (docCount > float64(len(hitsList)))

				portsMap := make(map[int]bool)
				servicesMap := make(map[string]bool)
				titlesMap := make(map[string]bool)
				domainsMap := make(map[string]bool)
				org := ""
				asn := 0
				os := ""
				isIPv6 := false
				var firstSeen, lastSeen time.Time

				for _, hitItem := range hitsList {
					h, ok := hitItem.(map[string]any)
					if !ok {
						continue
					}
					src, ok := h["_source"].(map[string]any)
					if !ok {
						continue
					}

					if p, ok := src["port"].(float64); ok && p > 0 {
						portsMap[int(p)] = true
					}
					if s, ok := src["service"].(string); ok && s != "" {
						servicesMap[s] = true
					}
					if t, ok := src["title"].(string); ok && t != "" {
						titlesMap[t] = true
					}
					if d, ok := src["domain"].(string); ok && d != "" {
						domainsMap[d] = true
					}
					if n, ok := src["name"].(string); ok && n != "" {
						domainsMap[n] = true
					}
					if reg, ok := src["registrable_domain"].(string); ok && reg != "" {
						domainsMap[reg] = true
					}
					if hst, ok := src["host"].(string); ok && hst != "" && !strings.Contains(hst, ipStr) {
						domainsMap[hst] = true
					}
					if o, ok := src["org"].(string); ok && o != "" {
						org = o
					}
					if a, ok := src["asn"].(float64); ok && a > 0 {
						asn = int(a)
					}
					if sOS, ok := src["os"].(string); ok && sOS != "" {
						os = sOS
					}
					if v6, ok := src["is_ipv6"].(bool); ok && v6 {
						isIPv6 = true
					}
					if fsStr, ok := src["first_seen"].(string); ok {
						if t, err := time.Parse(time.RFC3339, fsStr); err == nil {
							if firstSeen.IsZero() || t.Before(firstSeen) {
								firstSeen = t
							}
						}
					}
					if lsStr, ok := src["last_seen"].(string); ok {
						if t, err := time.Parse(time.RFC3339, lsStr); err == nil {
							if lastSeen.IsZero() || t.After(lastSeen) {
								lastSeen = t
							}
						}
					}
				}

				ports := make([]int, 0, len(portsMap))
				for p := range portsMap {
					ports = append(ports, p)
				}
				sort.Ints(ports)

				services := make([]string, 0, len(servicesMap))
				for s := range servicesMap {
					services = append(services, s)
				}
				sort.Strings(services)

				titles := make([]string, 0, len(titlesMap))
				for t := range titlesMap {
					titles = append(titles, t)
				}
				sort.Strings(titles)

				domains := make([]string, 0, len(domainsMap))
				for d := range domainsMap {
					domains = append(domains, d)
				}
				sort.Strings(domains)

				firstDom := ""
				if len(domains) > 0 {
					firstDom = domains[0]
				}

				item := map[string]any{
					"ip":                ipStr,
					"doc_type":          "host",
					"aggregated":        true,
					"open_ports":        ports,
					"services":          services,
					"titles":            titles,
					"domains":           domains,
					"domain":            firstDom,
					"org":               org,
					"asn":               asn,
					"os":                os,
					"is_ipv6":           isIPv6,
					"truncated":         isTruncated,
					"document_count":    int(docCount),
					"aggregation_limit": store.MaxTopHitsSize,
				}
				if !firstSeen.IsZero() {
					item["first_seen"] = firstSeen.Format(time.RFC3339)
				}
				if !lastSeen.IsZero() {
					item["last_seen"] = lastSeen.Format(time.RFC3339)
				}
				aggregatedItems = append(aggregatedItems, item)
			}

			afterKeyObj, hasAfter := compObj["after_key"].(map[string]any)
			if !hasAfter || len(afterKeyObj) == 0 {
				break
			}
			ipAfterKey = afterKeyObj
		}
	}

	// 2. 循环遍历纯域名桶
	if !skipDomainAgg {
		var domAfterKey map[string]any
		for {
			compQuery := store.BuildESCompositeQuery(root, kind, true, domAfterKey, 1000)
			rawResp, err := s.es.SearchAgg(ctx, compQuery)
			if err != nil {
				return nil, err
			}
			aggs, _ := rawResp["aggregations"].(map[string]any)
			if aggs == nil {
				break
			}
			compObj, _ := aggs["domain_composite"].(map[string]any)
			if compObj == nil {
				break
			}

			buckets, _ := compObj["buckets"].([]any)
			for _, b := range buckets {
				bMap, ok := b.(map[string]any)
				if !ok {
					continue
				}
				keyMap, _ := bMap["key"].(map[string]any)
				dKey, _ := keyMap["domain"].(string)
				if dKey == "" {
					dKey, _ = bMap["key"].(string)
				}

				topDocs, _ := bMap["top_docs"].(map[string]any)
				hitsObj, _ := topDocs["hits"].(map[string]any)
				hitsList, _ := hitsObj["hits"].([]any)
				if len(hitsList) == 0 {
					continue
				}
				firstHit, _ := hitsList[0].(map[string]any)
				src, _ := firstHit["_source"].(map[string]any)
				if src == nil {
					src = make(map[string]any)
				}

				dName, _ := src["name"].(string)
				dDomain, _ := src["domain"].(string)
				dReg, _ := src["registrable_domain"].(string)
				if dDomain == "" {
					dDomain = dKey
				}
				if dName == "" {
					dName = dDomain
				}

				item := map[string]any{
					"domain":             dDomain,
					"name":               dName,
					"registrable_domain": dReg,
					"doc_type":           "domain",
					"aggregated":         true,
					"resolved_ips":       src["resolved_ips"],
					"truncated":          false,
				}
				aggregatedItems = append(aggregatedItems, item)
			}

			afterKeyObj, hasAfter := compObj["after_key"].(map[string]any)
			if !hasAfter || len(afterKeyObj) == 0 {
				break
			}
			domAfterKey = afterKeyObj
		}
	}

	// 3. 100% 精确组总数计算
	total := int64(len(aggregatedItems))
	totalPages := 0
	if tp > 0 && total > 0 {
		totalPages = int((total + int64(tp) - 1) / int64(tp))
	}

	// 4. 绝对安全的范围切片
	from := (page - 1) * tp
	if from > int(total) {
		from = int(total)
	}
	to := from + tp
	if to > int(total) {
		to = int(total)
	}
	pageItems := aggregatedItems[from:to]

	return &store.SearchResult{
		Total:      total,
		Page:       page,
		PageSize:   tp,
		TotalPages: totalPages,
		Aggregated: true,
		Items:      pageItems,
	}, nil
}
