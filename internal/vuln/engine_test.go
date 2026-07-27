package vuln

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func fakeResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestMatchMatchers(t *testing.T) {
	resp := fakeResp(200, "hello [core] world")
	ms := []Matcher{
		{Type: "status", Status: 200},
		{Type: "word", Words: []string{"[core]"}, Condition: "and"},
	}
	if !matchMatchers(ms, resp, "hello [core] world") {
		t.Error("expected match")
	}

	// 状态不符
	resp2 := fakeResp(404, "[core]")
	if matchMatchers(ms, resp2, "[core]") {
		t.Error("expected no match on 404")
	}
}

func TestParseAndExtract(t *testing.T) {
	yaml := `
id: t1
info:
  name: "Test"
  severity: high
requests:
  - method: GET
    path: "/x"
    matchers:
      - type: word
        words: ["token="]
    extractors:
      - type: regex
        regex: "token=([a-z0-9]+)"
        part: body
        name: token
`
	tpl, err := ParseTemplate([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if tpl.Info.Severity != "high" || severityLevel("high") != 4 {
		t.Error("severity mapping wrong")
	}
	resp := fakeResp(200, "token=abc123")
	ev := extract(tpl.Requests[0].Extractors, resp, "token=abc123")
	if ev["token"] != "abc123" {
		t.Errorf("expected extract token=abc123, got %v", ev)
	}
	if !matchMatchers(tpl.Requests[0].Matchers, resp, "token=abc123") {
		t.Error("expected word match")
	}
}
