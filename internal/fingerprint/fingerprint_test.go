package fingerprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomRuleMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	rules := `
- name: "CustomMarker"
  body_re: "X-Marker: yes"
- name: "Baota"
  header: "Server"
  header_re: "(?i)btpanel"
- name: "SSH-OpenSSH"
  banner_re: "(?i)SSH-2.0-OpenSSH"
`
	if err := os.WriteFile(path, []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	// 命中 body 规则
	techs := s.Detect(map[string][]string{}, []byte("hello X-Marker: yes world"), "")
	if !contains(techs, "CustomMarker") {
		t.Errorf("expected CustomMarker, got %v", techs)
	}

	// 命中 header 规则
	techs = s.Detect(map[string][]string{"Server": {"btpanel/1.0"}}, []byte(""), "")
	if !contains(techs, "Baota") {
		t.Errorf("expected Baota, got %v", techs)
	}

	// 命中 banner 规则（TCP 横幅）
	techs = s.Detect(map[string][]string{}, []byte(""), "SSH-2.0-OpenSSH_8.2p1")
	if !contains(techs, "SSH-OpenSSH") {
		t.Errorf("expected SSH-OpenSSH, got %v", techs)
	}

	// 社区库：Apache Server 头
	techs = s.Detect(map[string][]string{"Server": {"Apache/2.4.29"}}, []byte(""), "")
	if !contains(techs, "Apache HTTP Server") {
		t.Errorf("expected Apache HTTP Server from wappalyzer, got %v", techs)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
