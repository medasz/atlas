package scan

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"atlas/internal/assetstore"
	"atlas/internal/config"
	"atlas/internal/fingerprint"
	"atlas/internal/logger"
	"atlas/internal/model"
	"atlas/internal/ratelimit"
	"atlas/internal/scan/tcpscan"
	"atlas/internal/store"
	"golang.org/x/net/publicsuffix"
)

// Scanner 资产探测引擎，实现 task.Processor
type Scanner struct {
	asset        assetstore.AssetStore
	pg           *store.Store
	rate         *ratelimit.Limiter
	fp           *fingerprint.Service
	defaultPorts []int
	timeout      time.Duration
	connSem      int

	enrichChan chan openPortEvent
	writerChan chan model.Asset
	cancel     context.CancelFunc

	// scanCfg 为运行时可热更新的扫描配置（模式/网卡/raw 参数）。
	// 通过 SetScanConfig 由配置 API 推送更新，scanHost 执行时实时读取，
	// 因此界面改模式/网卡无需重启即对新建任务生效。
	mu      sync.RWMutex
	scanCfg config.ScanConfig
}

// New 构造扫描器（fp 可为 nil，nil 时不做技术指纹识别）。
// scanCfg 提供扫描模式与 raw 相关配置（按值拷贝，后续由 SetScanConfig 热更新）。
func New(asset assetstore.AssetStore, r *ratelimit.Limiter, defaultPorts []int, fp *fingerprint.Service, scanCfg config.ScanConfig) *Scanner {
	if len(defaultPorts) == 0 {
		defaultPorts = TopPorts
	}
	sc := &Scanner{
		asset:        asset,
		rate:         r,
		fp:           fp,
		defaultPorts: defaultPorts,
		timeout:      1500 * time.Millisecond,
		connSem:      200,
		scanCfg:      scanCfg,
	}
	sc.startPipeline(context.Background())
	return sc
}

// Close 优雅关闭扫描器后台流水线，通知 Worker 退出并刷盘
func (sc *Scanner) Close() {
	if sc.cancel != nil {
		sc.cancel()
	}
}

// SetStore 注入 PostgreSQL 存储（用于持久化主机存活与生命周期数据，不污染 ES 资产库）。
func (sc *Scanner) SetStore(st *store.Store) {
	sc.pg = st
}

// SetScanConfig 运行时热更新扫描配置：界面改模式/网卡后无需重启即对新建任务生效。
func (sc *Scanner) SetScanConfig(cfg config.ScanConfig) {
	sc.mu.Lock()
	sc.scanCfg = cfg
	sc.mu.Unlock()
}

// liveScanCfg 读取当前生效的扫描配置（受 RWMutex 保护，避免与 SetScanConfig 竞争）。
func (sc *Scanner) liveScanCfg() config.ScanConfig {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.scanCfg
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

// scanHost 对 IP 目标做 TCP 端口扫描 + 服务/HTTP 探测。
// 按配置模式分派：raw 模式（SYN/ACK/FIN/Null/Xmas）整块广发 + 窗口抓包；
// connect 模式逐端口 goroutine 全连接（保留原限速 + panic 安全网）。
// raw 抓包不可用时自动降级为 connect。
func (sc *Scanner) scanHost(ctx context.Context, ip string, ports []int) (map[string]any, error) {
	// 实时读取扫描配置：运行时通过 SetScanConfig 热更新后，此处立即生效（无需重启）。
	live := sc.liveScanCfg()
	isV6 := isIPv6(ip)
	recordFiltered := live.RecordFilteredPorts
	recordClosed := live.RecordClosedPorts

	var (
		mu        sync.Mutex
		openPorts []int
	)
	logger.Debug("开始探测目标", "target", ip, "ports_count", len(ports), "mode", live.DefaultMode)

	if isV6 {
		// IPv6 能力边界：raw 抓包/ICMPv6 判定与 IPv4 不同，本期强制降级 connect 并记日志。
		log.Printf("tcpscan: IPv6 目标 %s 强制使用 connect（raw 暂不支持 IPv6）", ip)
		res, err := sc.connectFallback(ctx, ip, ports)
		if err != nil {
			return nil, err
		}
		for _, p := range ports {
			if r, ok := res[p]; ok {
				sc.persistResult(ctx, ip, p, r, isV6, recordFiltered, recordClosed, &mu, &openPorts)
			}
		}
		return sc.finishHost(ctx, ip, isV6, openPorts)
	}

	mode := tcpscan.Mode(live.DefaultMode)
	if mode == "" {
		mode = tcpscan.ModeConnect
	}

	if mode.IsRaw() {
		// raw 批量：整个 IP 的端口块一次性发出 + 一个抓包窗口回收响应，
		// 速率限制在每个目标 IP 维度各触发一次（与 connect 路径语义一致）。
		_ = sc.rate.WaitGlobal(ctx)
		_ = sc.rate.WaitTarget(ctx, ip)
		opts := tcpscan.Options{
			Timeout:        time.Duration(live.RawCaptureWindowSec) * time.Second,
			Retries:        live.RawRetries,
			Iface:          live.RawIface,
			InstallRstDrop: live.InstallRstDrop,
		}
		ts, err := tcpscan.New(mode, opts)
		if err != nil {
			// 非法模式：回退 connect 路径
			log.Printf("tcpscan: 非法模式 %q，回退 connect: %v", mode, err)
			mode = tcpscan.ModeConnect
		} else {
			var res map[int]tcpscan.Result
			var scanErr error
			// recover 兜底：保证单目标 raw 扫描 panic 不拖垮整个 worker。
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("scan: recovered panic in raw scan ip=%s: %v\n%s", ip, r, debug.Stack())
						scanErr = fmt.Errorf("raw scan panic: %v", r)
					}
				}()
				res, scanErr = ts.Scan(ctx, ip, ports, opts)
			}()
			if scanErr != nil {
				if tcpscan.IsRawUnavailable(scanErr) {
					log.Printf("tcpscan: raw 抓包不可用，降级为 connect 扫描 %s: %v", ip, scanErr)
					res, scanErr = sc.connectFallback(ctx, ip, ports)
				}
				if scanErr != nil {
					return nil, scanErr
				}
			}
			for _, p := range ports {
				if r, ok := res[p]; ok {
					sc.persistResult(ctx, ip, p, r, isV6, recordFiltered, recordClosed, &mu, &openPorts)
				}
			}
			return sc.finishHost(ctx, ip, isV6, openPorts)
		}
	}

	// connect 模式（含非法模式回退）：逐端口 goroutine（保留原限速 + panic 安全网）
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

			logger.Info("发现开放端口", "target", ip, "port", p, "banner", banner)

			if sc.enrichChan != nil {
				sc.enrichChan <- openPortEvent{IP: ip, Port: p, Banner: banner, IsV6: isV6}
			} else {
				portAsset := model.Asset{
					IP:       ip,
					Port:     p,
					Proto:    "tcp",
					State:    string(tcpscan.Open),
					Service:  guessService(p, banner),
					Banner:   banner,
					Host:     ip,
					IsIPv6:   isV6,
					LastSeen: time.Now(),
				}
				sc.httpEnrich(ip, p, banner, &portAsset)
				sc.upsert(ctx, portAsset)
			}

			mu.Lock()
			openPorts = append(openPorts, p)
			mu.Unlock()
		}(port)
	}
	wg.Wait()
	return sc.finishHost(ctx, ip, isV6, openPorts)
}

// finishHost 将主机在线打卡与生命周期落库至 PostgreSQL（不再向 ES 写入 port:<ip>:0 垃圾数据）并返回扫描结果摘要。
func (sc *Scanner) finishHost(ctx context.Context, ip string, isV6 bool, openPorts []int) (map[string]any, error) {
	if sc.pg != nil {
		if err := sc.pg.UpsertIPLifecycle(ctx, ip, isV6, len(openPorts)); err != nil {
			log.Printf("scan: upsert ip lifecycle %s to pg failed: %v", ip, err)
		}
	}
	return map[string]any{"ip": ip, "open_ports": openPorts, "count": len(openPorts)}, nil
}


// shouldPersist 依据配置决定某端口状态是否落库：
//   - open 始终落库（确认的开放端口）；
//   - filtered / open|filtered / unfiltered 受 RecordFilteredPorts 控制（默认 true），
//     即「不确定/拓扑类」结果默认落库，closed/timeout 不在此列；
//   - closed / timeout 受 RecordClosedPorts 控制（默认 false，防全端口扫描 PG 膨胀）。
func shouldPersist(state string, recordFiltered, recordClosed bool) bool {
	switch tcpscan.State(state) {
	case tcpscan.Open:
		return true
	case tcpscan.Filtered, tcpscan.OpenFiltered, tcpscan.Unfiltered:
		return recordFiltered
	case tcpscan.Closed, tcpscan.Timeout:
		return recordClosed
	}
	return false
}

// persistResult 按持久化策略将单端口扫描结果落库。
func (sc *Scanner) persistResult(ctx context.Context, ip string, p int, r tcpscan.Result, isV6 bool, recordFiltered, recordClosed bool, mu *sync.Mutex, openPorts *[]int) {
	if !shouldPersist(string(r.State), recordFiltered, recordClosed) {
		return
	}
	portAsset := model.Asset{
		IP:      ip,
		Port:    p,
		Proto:   "tcp",
		State:   string(r.State),
		Service: guessService(p, r.Banner),
		Banner:  r.Banner,
		Host:    ip,
		IsIPv6:  isV6,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}
	if r.State == tcpscan.Open {
		if sc.enrichChan != nil {
			sc.enrichChan <- openPortEvent{IP: ip, Port: p, Banner: r.Banner, IsV6: isV6}
		} else {
			sc.httpEnrich(ip, p, r.Banner, &portAsset)
			sc.upsert(ctx, portAsset)
		}
	} else {
		sc.upsert(ctx, portAsset)
	}

	mu.Lock()
	if r.State == tcpscan.Open {
		*openPorts = append(*openPorts, p)
	}
	mu.Unlock()
}

// httpEnrich 对满足 HTTP 条件的端口做 HTTP(S) 探测并回填标题/技术栈/证书。
func (sc *Scanner) httpEnrich(ip string, p int, banner string, portModel *model.Asset) {
	if !(commonHTTPPorts[p] || looksLikeHTTP(banner)) {
		return
	}
	hr, err := httpProbe(ip, p, sc.timeout)
	if err != nil {
		return
	}
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

// upsert 写入资产；单条失败仅记日志不中断扫描（与 panic 安全网一致，避免单端口写入异常拖垮整个目标）。
func (sc *Scanner) upsert(ctx context.Context, a model.Asset) {
	if err := sc.asset.Upsert(ctx, a); err != nil {
		log.Printf("scan: upsert %s failed: %v", model.AssetID(a), err)
	}
}

// connectFallback raw 抓包不可用时的降级：逐端口全连接，仅能判定 open/closed。
func (sc *Scanner) connectFallback(ctx context.Context, ip string, ports []int) (map[int]tcpscan.Result, error) {
	out := make(map[int]tcpscan.Result, len(ports))
	for _, p := range ports {
		conn, err := tcpConnect(ip, p, sc.timeout)
		if err != nil {
			out[p] = tcpscan.Result{Port: p, State: tcpscan.Closed}
			continue
		}
		banner := grabBanner(conn, sc.timeout)
		_ = conn.Close()
		out[p] = tcpscan.Result{Port: p, State: tcpscan.Open, Banner: banner}
	}
	return out, nil
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
		portAsset := model.Asset{
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
			portAsset.Cert = hr.Cert
		}
		sc.upsert(ctx, portAsset)
		open = append(open, webinfo)
	}
	if err := sc.asset.Upsert(ctx, model.Asset{
		Domain:            domain,
		Host:              domain,
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
