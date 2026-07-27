package fingerprint

import (
	"fmt"
	"os"
	"regexp"
	"sync"

	wappalyzergo "github.com/projectdiscovery/wappalyzergo"
	"gopkg.in/yaml.v3"
)

// Rule 自研指纹规则（热加载）
type Rule struct {
	Name      string `yaml:"name"`
	Header    string `yaml:"header"`     // 需匹配的响应头名（可选，如 Set-Cookie）
	HeaderRe  string `yaml:"header_re"`  // 响应头值正则（可选）
	BodyRe    string `yaml:"body_re"`    // 响应体正则（可选，可覆盖 title/body 特征）
	Banner    string `yaml:"banner"`     // 匹配 TCP 原始 banner 正则（可选）
	BannerRe  string `yaml:"banner_re"`  // 同 banner
	compiled  compiledRule
}

type compiledRule struct {
	headerRe *regexp.Regexp
	bodyRe   *regexp.Regexp
	bannerRe *regexp.Regexp
}

// Service 技术指纹识别：wappalyzer 社区库 + 自研规则
type Service struct {
	wapp *wappalyzergo.Wappalyze
	mu   sync.RWMutex
	rules []Rule
	path string
}

// New 构造指纹服务并加载自研规则（path 为空则仅用社区库）
func New(path string) (*Service, error) {
	w, err := wappalyzergo.New()
	if err != nil {
		return nil, fmt.Errorf("init wappalyzer: %w", err)
	}
	s := &Service{wapp: w, path: path}
	if path != "" {
		if err := s.Load(path); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Load 读取并编译自研规则文件
func (s *Service) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read rules %s: %w", path, err)
	}
	var rules []Rule
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("parse rules %s: %w", path, err)
	}
	for i := range rules {
		if rules[i].Name == "" {
			return fmt.Errorf("rule #%d missing name", i)
		}
		r := &rules[i]
		if r.HeaderRe != "" {
			re, err := regexp.Compile(r.HeaderRe)
			if err != nil {
				return fmt.Errorf("rule %q header_re: %w", r.Name, err)
			}
			r.compiled.headerRe = re
		}
		if r.BodyRe != "" {
			re, err := regexp.Compile(r.BodyRe)
			if err != nil {
				return fmt.Errorf("rule %q body_re: %w", r.Name, err)
			}
			r.compiled.bodyRe = re
		}
		if r.BannerRe != "" {
			re, err := regexp.Compile(r.BannerRe)
			if err != nil {
				return fmt.Errorf("rule %q banner_re: %w", r.Name, err)
			}
			r.compiled.bannerRe = re
		}
	}
	s.mu.Lock()
	s.rules = rules
	s.path = path
	s.mu.Unlock()
	return nil
}

// Reload 热加载：重新读取规则文件
func (s *Service) Reload() error {
	if s.path == "" {
		return fmt.Errorf("no rules file configured")
	}
	return s.Load(s.path)
}

// Detect 识别技术栈，返回去重后的技术名列表。
// banner 为 TCP 原始横幅（可选），用于匹配非 HTTP 服务指纹。
func (s *Service) Detect(headers map[string][]string, body []byte, banner string) []string {
	out := map[string]struct{}{}
	for name := range s.wapp.Fingerprint(headers, body) {
		out[name] = struct{}{}
	}
	s.mu.RLock()
	rules := s.rules
	s.mu.RUnlock()
	for _, r := range rules {
		if matchRule(r, headers, body, banner) {
			out[r.Name] = struct{}{}
		}
	}
	names := make([]string, 0, len(out))
	for k := range out {
		names = append(names, k)
	}
	return names
}

func matchRule(r Rule, headers map[string][]string, body []byte, banner string) bool {
	if r.compiled.headerRe != nil && r.Header != "" {
		for _, v := range headers[r.Header] {
			if r.compiled.headerRe.MatchString(v) {
				return true
			}
		}
	}
	if r.compiled.bodyRe != nil {
		if r.compiled.bodyRe.Match(body) {
			return true
		}
	}
	if r.compiled.bannerRe != nil && r.compiled.bannerRe.MatchString(banner) {
		return true
	}
	return false
}
