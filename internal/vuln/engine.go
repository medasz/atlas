package vuln

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"atlas/internal/model"
	"atlas/internal/ratelimit"
	"atlas/internal/store"
)

// Engine 漏洞检测引擎（自研，nuclei 风格模板，仅验证不利用）
type Engine struct {
	store     *store.Store
	rate      *ratelimit.Limiter
	mu        sync.RWMutex
	templates []Template
	timeout   time.Duration
	client    *http.Client
}

// New 构造引擎并加载模板目录
func New(s *store.Store, r *ratelimit.Limiter, templatesDir string) (*Engine, error) {
	e := &Engine{
		store:   s,
		rate:    r,
		timeout: 5 * time.Second,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
			CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	if templatesDir != "" {
		ts, err := LoadTemplatesDir(templatesDir)
		if err != nil {
			return nil, err
		}
		e.templates = ts
	}
	return e, nil
}

// Templates 返回当前加载的模板（供 API 展示）
func (e *Engine) Templates() []Template {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.templates
}

// AddTemplate 运行时新增模板（API 上传）
func (e *Engine) AddTemplate(t Template) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.templates = append(e.templates, t)
}

// Process 实现 task.Processor：对目标执行全部 HTTP 模板
func (e *Engine) Process(ctx context.Context, task model.Task, target, _ string) (map[string]any, error) {
	found := []map[string]any{}
	ports := []struct {
		port   int
		scheme string
	}{
		{80, "http"}, {8080, "http"}, {8000, "http"}, {8888, "http"},
		{443, "https"}, {8443, "https"},
	}
	for _, p := range ports {
		base := p.scheme + "://" + net.JoinHostPort(target, strconv.Itoa(p.port))
		for _, t := range e.currentTemplates() {
			for _, req := range t.Requests {
				_ = e.rate.WaitGlobal(ctx)
				vuln, matched := e.runTemplate(ctx, target, base, t, req)
				if matched {
					_ = e.store.UpsertVuln(ctx, vuln)
					found = append(found, map[string]any{"kpid": vuln.KPID, "name": vuln.Name, "level": vuln.Level, "asset": vuln.AssetRef})
				}
			}
		}
	}
	return map[string]any{"target": target, "found": found, "count": len(found)}, nil
}

func (e *Engine) currentTemplates() []Template {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.templates
}

func (e *Engine) runTemplate(ctx context.Context, target, base string, t Template, req HTTPRequest) (model.Vuln, bool) {
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	url := base + req.Path
	httpReq, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return model.Vuln{}, false
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", "Atlas-Scanner/0.1")
	}
	resp, err := e.client.Do(httpReq)
	if err != nil {
		return model.Vuln{}, false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))

	if !matchMatchers(req.Matchers, resp, string(body)) {
		return model.Vuln{}, false
	}
	evidence := extract(req.Extractors, resp, string(body))
	v := model.Vuln{
		AssetRef:     target,
		KPID:         t.ID,
		CVE:          t.Info.CVE,
		Name:         t.Info.Name,
		Level:        severityLevel(t.Info.Severity),
		Type:         "http",
		Proof:        proofString(t, req, evidence),
		Status:       "open",
		FirstFound:   time.Now(),
		LastVerified: time.Now(),
	}
	return v, true
}

func proofString(t Template, req HTTPRequest, evidence map[string]string) string {
	proof := "matched template " + t.ID + " on " + req.Path
	if len(evidence) > 0 {
		proof += " | evidence: " + mapToStr(evidence)
	}
	return proof
}

func mapToStr(m map[string]string) string {
	s := ""
	for k, v := range m {
		s += k + "=" + v + ";"
	}
	return s
}

func severityLevel(sev string) int {
	switch sev {
	case "info":
		return 1
	case "low":
		return 2
	case "medium":
		return 3
	case "high":
		return 4
	case "critical":
		return 5
	}
	return 3
}
