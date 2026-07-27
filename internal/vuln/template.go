package vuln

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Template nuclei 风格检测模板（MVP 支持 http 请求 + 匹配器/提取器）
type Template struct {
	ID       string       `yaml:"id"`
	Info     Info         `yaml:"info"`
	Requests []HTTPRequest `yaml:"requests"`
}

// Info 模板元信息
type Info struct {
	Name     string `yaml:"name"`
	Severity string `yaml:"severity"`
	Tags     string `yaml:"tags"`
	CVE      string `yaml:"cve"`
}

// HTTPRequest HTTP 检测请求
type HTTPRequest struct {
	Method    string            `yaml:"method"`
	Path      string            `yaml:"path"`
	Headers   map[string]string `yaml:"headers"`
	Matchers  []Matcher         `yaml:"matchers"`
	Extractors []Extractor      `yaml:"extractors"`
}

// Matcher 匹配器
type Matcher struct {
	Type      string   `yaml:"type"`      // word | status | regex
	Part      string   `yaml:"part"`      // status | header | body
	Words     []string `yaml:"words"`     // word 匹配关键字
	Regex     []string `yaml:"regex"`     // regex 模式
	Status    int      `yaml:"status"`    // status 匹配码
	Condition string   `yaml:"condition"` // and | or（多个 matcher 之间）
	Name      string   `yaml:"name"`
}

// Extractor 提取器（用于收集证据）
type Extractor struct {
	Type  string `yaml:"type"` // regex
	Regex string `yaml:"regex"`
	Part  string `yaml:"part"` // header | body
	Name  string `yaml:"name"`
}

// ParseTemplate 解析单个 YAML 模板
func ParseTemplate(data []byte) (Template, error) {
	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("parse template: %w", err)
	}
	if t.ID == "" {
		return t, fmt.Errorf("template missing id")
	}
	if len(t.Requests) == 0 {
		return t, fmt.Errorf("template %q has no requests", t.ID)
	}
	return t, nil
}

// LoadTemplatesDir 加载目录下全部 .yaml 模板
func LoadTemplatesDir(dir string) ([]Template, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", err)
	}
	out := []Template{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		t, err := ParseTemplate(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, t)
	}
	return out, nil
}
