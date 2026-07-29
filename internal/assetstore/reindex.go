package assetstore

import (
	"context"
	"log"

	"atlas/internal/model"
	"atlas/internal/store"
)

// ReindexFromPG 把 PG 中的资产全量写入 AssetStore（ES）。
// 仅在删 PG 资产表前的一次性迁移使用；Task 10 删表后随 PG 资产读方法一并移除。
func ReindexFromPG(ctx context.Context, pg *store.Store, a AssetStore) error {
	hosts, err := pg.ListAllHosts(ctx)
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if err := a.Upsert(ctx, model.Asset{
			Kind:      model.KindHost,
			IP:        h.IP,
			ASN:       h.ASN,
			Org:       h.Org,
			OS:        h.OS,
			IsIPv6:    h.IsIPv6,
			OpenPorts: len(h.OpenPorts),
			Geo:       h.Geo,
			FirstSeen: h.FirstSeen,
			LastSeen:  h.LastSeen,
		}); err != nil {
			log.Printf("reindex host %s: %v", h.IP, err)
		}
	}

	ports, err := pg.ListAllPorts(ctx)
	if err != nil {
		return err
	}
	for _, p := range ports {
		if err := a.Upsert(ctx, model.Asset{
			Kind:      model.KindPort,
			IP:        p.IP,
			Port:      p.Port,
			Proto:     p.Proto,
			State:     p.State,
			Service:   p.Service,
			Version:   p.Version,
			Banner:    p.Banner,
			Title:     p.Title,
			Host:      p.Host,
			IsIPv6:    p.IsIPv6,
			Cert:      p.Cert,
			WebInfo:   p.WebInfo,
			FirstSeen: p.FirstSeen,
			LastSeen:  p.LastSeen,
		}); err != nil {
			log.Printf("reindex port %s:%d: %v", p.IP, p.Port, err)
		}
	}

	domains, err := pg.ListAllDomains(ctx)
	if err != nil {
		return err
	}
	for _, d := range domains {
		if err := a.Upsert(ctx, model.Asset{
			Kind:              model.KindDomain,
			Domain:            d.Name,
			Host:              d.Name,
			RegistrableDomain: d.RegistrableDomain,
			ResolvedIPs:       d.ResolvedIPs,
			CNAME:             d.CNAME,
			Org:               d.Org,
			ASN:               d.ASN,
			IsIPv6:            d.IsIPv6,
			Whois:             d.Whois,
			FirstSeen:         d.FirstSeen,
			LastSeen:          d.LastSeen,
		}); err != nil {
			log.Printf("reindex domain %s: %v", d.Name, err)
		}
	}

	log.Printf("reindex done: %d hosts, %d ports, %d domains", len(hosts), len(ports), len(domains))
	return nil
}
