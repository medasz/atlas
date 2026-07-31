package model

import (
	"strconv"
	"time"
)

// Asset 资产本体统一结构。
// 端口必须仍是独立文档（满足 port 维度列表与 port=22 检索），不并入 host。
// 不同类型（host/port/domain）以 _id 前缀区分，不再依赖分类字段。
type Asset struct {
	IP                string         `json:"ip,omitempty"`
	Port              int            `json:"port,omitempty"`
	Proto             string         `json:"proto,omitempty"`
	Domain            string         `json:"domain,omitempty"` // domain kind 的完整主机名（=原 Domain.Name）
	Host              string         `json:"host,omitempty"`   // 到达端口所用主机名/域名（HTTP Host）
	ASN               int            `json:"asn,omitempty"`
	Org               string         `json:"org,omitempty"`
	OS                string         `json:"os,omitempty"`
	IsIPv6            bool           `json:"is_ipv6,omitempty"`
	State             string         `json:"state,omitempty"`
	Service           string         `json:"service,omitempty"`
	Version           string         `json:"version,omitempty"`
	Banner            string         `json:"banner,omitempty"`
	Title             string         `json:"title,omitempty"`
	Server            string         `json:"server,omitempty"`
	Tech              []string       `json:"tech,omitempty"`
	RegistrableDomain string         `json:"registrable_domain,omitempty"`
	ResolvedIPs       []string       `json:"resolved_ips,omitempty"`
	CNAME             []string       `json:"cname,omitempty"`
	OpenPorts         int            `json:"open_ports,omitempty"`
	Cert              map[string]any `json:"cert,omitempty"`
	WebInfo           map[string]any `json:"webinfo,omitempty"`
	Geo               map[string]any `json:"geo,omitempty"`
	Whois             map[string]any `json:"whois,omitempty"`
	FirstSeen         time.Time      `json:"first_seen,omitempty"`
	LastSeen          time.Time      `json:"last_seen,omitempty"`
}

// AssetID 返回单文档模型下的 ES _id：port:<ip>:<port>。
func AssetID(a Asset) string {
	if a.Port != 0 {
		return "port:" + a.IP + ":" + strconv.Itoa(a.Port)
	}
	if a.IP != "" {
		return "port:" + a.IP + ":0"
	}
	if a.Domain != "" {
		return "port:" + a.Domain + ":0"
	}
	return "port:unknown:0"
}
