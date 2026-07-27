package vuln

import (
	"net/http"
	"regexp"
	"strings"
)

// partContent 取匹配目标文本（status/header/body）
func partContent(part string, resp *http.Response, body string) string {
	switch part {
	case "status":
		return resp.Status
	case "header":
		var b strings.Builder
		for k, vs := range resp.Header {
			b.WriteString(k)
			b.WriteString(":")
			b.WriteString(strings.Join(vs, " "))
			b.WriteString("\n")
		}
		return b.String()
	default: // body
		return body
	}
}

// matchOne 评估单个匹配器
func matchOne(m Matcher, resp *http.Response, body string) bool {
	content := partContent(m.Part, resp, body)
	switch m.Type {
	case "status":
		return resp.StatusCode == m.Status
	case "word":
		for _, w := range m.Words {
			if !strings.Contains(strings.ToLower(content), strings.ToLower(w)) {
				return false
			}
		}
		return len(m.Words) > 0
	case "regex":
		for _, pat := range m.Regex {
			re, err := regexp.Compile(pat)
			if err != nil {
				continue
			}
			if re.MatchString(content) {
				return true
			}
		}
		return false
	}
	return false
}

// matchMatchers 按 condition 组合多个匹配器（默认 or）
func matchMatchers(matchers []Matcher, resp *http.Response, body string) bool {
	if len(matchers) == 0 {
		return false
	}
	cond := "or"
	for _, m := range matchers {
		if m.Condition == "and" {
			cond = "and"
			break
		}
	}
	for _, m := range matchers {
		ok := matchOne(m, resp, body)
		if cond == "or" && ok {
			return true
		}
		if cond == "and" && !ok {
			return false
		}
	}
	return cond == "and"
}

// extract 运行提取器，返回 name->value 证据
func extract(exts []Extractor, resp *http.Response, body string) map[string]string {
	out := map[string]string{}
	for _, e := range exts {
		if e.Type != "regex" || e.Regex == "" {
			continue
		}
		content := partContent(e.Part, resp, body)
		re, err := regexp.Compile(e.Regex)
		if err != nil {
			continue
		}
		if m := re.FindStringSubmatch(content); m != nil {
			val := m[0]
			if len(m) > 1 {
				val = m[1]
			}
			key := e.Name
			if key == "" {
				key = "extract"
			}
			out[key] = val
		}
	}
	return out
}
