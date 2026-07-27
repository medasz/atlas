package scan

import (
	"context"
	"log"
	"net"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"atlas/internal/fingerprint"
	"atlas/internal/model"
	"atlas/internal/ratelimit"
	"atlas/internal/store"
	"golang.org/x/net/publicsuffix"
)

// Scanner 资产探测引擎，实现 task.Processor
type Scanner struct {
	store        *store.Store
	rate         *ratelimit.Limiter
	fp           *fingerprint.Service
	defaultPorts []int
	timeout      time.Duration
	connSem      int
}

// New 构造扫描器（fp 可为 nil，nil 时不做技术指纹识别）
func New(s *store.Store, r *ratelimit.Limiter, defaultPorts []int, fp *fingerprint.Service) *Scanner {
	if len(defaultPorts) == 0 {
		defaultPorts = TopPorts
	}
	return &Scanner{store: s, rate: r, fp: fp, defaultPorts: defaultPorts, timeout: 1500 * time.Millisecond, connSem: 50}
}

// Process 实现 task.Processor：根据目标类型分派
func (sc *Scanner) Process(ctx context.Context, task model.Task, target, ports string) (map[string]any, error) {
	var plist []int
	if ports != "" {
		if ps, err := ParsePortSpec(ports); err == nil && len(ps) > 0 {
			plist = ps
		}
	}
	if plist == nil {
		plist = sc.portsFor(task)
	}
	if net.ParseIP(target) != nil {
		return sc.scanHost(ctx, target, plist)
	}
	return sc.scanDomain(ctx, target, plist)
}

func (sc *Scanner) portsFor(task model.Task) []int {
	if v, ok := task.Scope["ports"].(string); ok {
		if ps, err := ParsePortSpec(v); err == nil && len(ps) > 0 {
			return ps
		}
	}
	return sc.defaultPorts
}

// scanHost 对 IP 目标做 TCP 端口扫描 + 服务/HTTP 探测
func (sc *Scanner) scanHost(ctx context.Context, ip string, ports []int) (map[string]any, error) {
	isV6 := isIPv6(ip)
	var (
		mu        sync.Mutex
		openPorts []int
		portsOut  []model.Port
	)
	sem := make(chan struct{}, sc.connSem)
	var wg sync.WaitGroup
	for _, port := range ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("scan: recovered panic ip=%s port=%d: %v\n%s", ip, p, r, debug.Stack())
				}
			}()
			sem <- struct{}{}
			defer func() { <-sem }()
			_ = sc.rate.WaitGlobal(ctx)
			_ = sc.rate.WaitTarget(ctx, ip)

			conn, err := tcpConnect(ip, p, sc.timeout)
			if err != nil {
				return
			}
			banner := grabBanner(conn, sc.timeout)
			_ = conn.Close()

			portModel := model.Port{
				IP:      ip,
				Port:    p,
				Proto:   "tcp",
				Service: guessService(p, banner),
				Banner:  banner,
				Host:    ip,
				IsIPv6:  isV6,
				FirstSeen: time.Now(),
				LastSeen:  time.Now(),
			}
			if commonHTTPPorts[p] || looksLikeHTTP(banner) {
				if hr, err := httpProbe(ip, p, sc.timeout); err == nil {
					portModel.Title = hr.Title
					webinfo := map[string]any{"status": hr.Status, "server": hr.Server, "x_powered_by": hr.XPoweredBy, "scheme": hr.Scheme}
					if sc.fp != nil {
						webinfo["tech"] = sc.fp.Detect(hr.Header, hr.Body, banner)
					}
					portModel.WebInfo = webinfo
					if hr.Cert != nil {
						portModel.Cert = hr.Cert
					}
				}
			}
			_ = sc.store.UpsertPort(ctx, portModel)

			mu.Lock()
			openPorts = append(openPorts, p)
			portsOut = append(portsOut, portModel)
			mu.Unlock()
		}(port)
	}
	wg.Wait()

	if err := sc.store.UpsertHost(ctx, model.Host{
		IP:        ip,
		OpenPorts: openPorts,
		IsIPv6:    isV6,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}); err != nil {
		return nil, err
	}
	return map[string]any{"ip": ip, "open_ports": openPorts, "count": len(openPorts)}, nil
}

// scanDomain 对域名目标做 HTTP(S) 探测
func (sc *Scanner) scanDomain(ctx context.Context, domain string, ports []int) (map[string]any, error) {
	open := []map[string]any{}
	for _, p := range ports {
		if !commonHTTPPorts[p] {
			continue
		}
		_ = sc.rate.WaitGlobal(ctx)
		hr, err := httpProbe(domain, p, sc.timeout)
		if err != nil {
			continue
		}
		webinfo := map[string]any{"status": hr.Status, "server": hr.Server, "x_powered_by": hr.XPoweredBy, "scheme": hr.Scheme}
	if sc.fp != nil {
		webinfo["tech"] = sc.fp.Detect(hr.Header, hr.Body, "")
	}
	portModel := model.Port{
		IP:      domain,
		Port:    p,
		Proto:   "tcp",
		Service: "http",
		Title:   hr.Title,
		Host:    domain,
		WebInfo: webinfo,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}
		if hr.Cert != nil {
			portModel.Cert = hr.Cert
		}
		_ = sc.store.UpsertPort(ctx, portModel)
		open = append(open, webinfo)
	}
	if err := sc.store.UpsertDomain(ctx, model.Domain{
		Name:              domain,
		RegistrableDomain: registrableDomain(domain),
		FirstSeen:         time.Now(),
		LastSeen:          time.Now(),
	}); err != nil {
		return nil, err
	}
	return map[string]any{"domain": domain, "web": open}, nil
}

// isIPv6 判断给定地址是否为 IPv6
func isIPv6(ip string) bool {
	p := net.ParseIP(ip)
	return p != nil && p.To4() == nil
}

// registrableDomain 提取注册根域（如 www.example.co.uk → example.co.uk）
func registrableDomain(name string) string {
	if i := strings.IndexAny(name, ":/"); i >= 0 {
		name = name[:i]
	}
	rd, err := publicsuffix.EffectiveTLDPlusOne(name)
	if err != nil {
		return name
	}
	return rd
}

func guessService(port int, banner string) string {
	switch port {
	case 21:
		return "ftp"
	case 22:
		return "ssh"
	case 23:
		return "telnet"
	case 25, 465, 587:
		return "smtp"
	case 53:
		return "dns"
	case 80, 8080, 8000, 8888:
		return "http"
	case 443, 8443:
		return "https"
	case 3306:
		return "mysql"
	case 5432:
		return "postgresql"
	case 6379:
		return "redis"
	case 27017:
		return "mongodb"
	case 9200:
		return "elasticsearch"
	case 11211:
		return "memcached"
	case 3389:
		return "rdp"
	case 445:
		return "smb"
	case 139:
		return "netbios"
	}
	if looksLikeHTTP(banner) {
		return "http"
	}
	return ""
}

func looksLikeHTTP(banner string) bool {
	return strings.HasPrefix(banner, "HTTP")
}

// TopPorts 常用端口默认集合
var TopPorts = []int{
	21, 22, 23, 25, 53, 80, 110, 111, 135, 139, 143, 443, 445, 465, 587, 993, 995,
	1433, 1521, 1723, 3306, 3389, 5432, 5900, 5985, 5986, 6379, 7001, 8080, 8443,
	8888, 9000, 9200, 11211, 27017, 27018, 27019,
}
