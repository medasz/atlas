package model

import (
	"strconv"
	"time"
)

// AssetKind 资产类型判别
type AssetKind string

const (
	KindHost   AssetKind = "host"
	KindPort   AssetKind = "port"
	KindDomain AssetKind = "domain"
)

// Asset 资产本体统一结构；doc_type 区分 host/port/domain。
// 端口必须仍是独立文档（满足 port 维度列表与 port=22 检索），不并入 host。
type Asset struct {
	Kind   AssetKind       `json:"doc_type"`
	IP     string          `json:"ip,omitempty"`
	Port   int             `json:"port,omitempty"`
	Proto  string          `json:"proto,omitempty"`
	Domain string          `json:"domain,omitempty"` // domain kind 的完整主机名（=原 Domain.Name）
	Host   string          `json:"host,omitempty"`   // 到达端口所用主机名/域名（HTTP Host）
	ASN    int             `json:"asn,omitempty"`
	Org    string          `json:"org,omitempty"`
	OS     string          `json:"os,omitempty"`
	IsIPv6 bool            `json:"is_ipv6,omitempty"`
	State  string          `json:"state,omitempty"`
	Service string         `json:"service,omitempty"`
	Version string         `json:"version,omitempty"`
	Banner string          `json:"banner,omitempty"`
	Title  string          `json:"title,omitempty"`
	Server string          `json:"server,omitempty"`
	Tech   []string        `json:"tech,omitempty"`
	RegistrableDomain string `json:"registrable_domain,omitempty"`
	ResolvedIPs []string   `json:"resolved_ips,omitempty"`
	CNAME  []string        `json:"cname,omitempty"`
	OpenPorts int          `json:"open_ports,omitempty"`
	Cert   map[string]any  `json:"cert,omitempty"`
	WebInfo map[string]any `json:"webinfo,omitempty"`
	Geo    map[string]any  `json:"geo,omitempty"`
	Whois  map[string]any  `json:"whois,omitempty"`
	FirstSeen time.Time    `json:"first_seen,omitempty"`
	LastSeen  time.Time    `json:"last_seen,omitempty"`
}

// AssetID 返回 ES _id：host:<ip> / port:<ip>:<port> / domain:<name>
func AssetID(a Asset) string {
	switch a.Kind {
	case KindPort:
		return "port:" + a.IP + ":" + strconv.Itoa(a.Port)
	case KindDomain:
		return "domain:" + a.Domain
	default:
		return "host:" + a.IP
	}
}
