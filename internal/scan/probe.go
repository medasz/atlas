package scan

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var titleRe = regexp.MustCompile(`(?m)<title[^>]*>(.*?)</title>`)

// tcpConnect 建立 TCP 连接（connect 扫描）
func tcpConnect(ip string, port int, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), timeout)
}

// grabBanner 连接后尝试读取横幅（限时）
func grabBanner(conn net.Conn, timeout time.Duration) string {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return ""
	}
	return strings.TrimSpace(string(buf[:n]))
}

// httpResult HTTP 探测结果
type httpResult struct {
	Scheme     string
	Status     int
	Title      string
	Server     string
	XPoweredBy string
	Cert       map[string]any
	Header     map[string][]string
	Body       []byte
}

// httpProbe 对目标端口发起 HTTP/HTTPS 探测
func httpProbe(host string, port int, timeout time.Duration) (httpResult, error) {
	scheme := "http"
	if port == 443 || port == 8443 {
		scheme = "https"
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	url := scheme + "://" + net.JoinHostPort(host, strconv.Itoa(port))
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return httpResult{}, err
	}
	req.Header.Set("User-Agent", "Atlas-Scanner/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return httpResult{}, err
	}
	defer resp.Body.Close()

	res := httpResult{Scheme: scheme, Status: resp.StatusCode}
	res.Server = resp.Header.Get("Server")
	res.XPoweredBy = resp.Header.Get("X-Powered-By")
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		c := resp.TLS.PeerCertificates[0]
		res.Cert = map[string]any{
			"subject_cn": c.Subject.CommonName,
			"issuer_cn":  c.Issuer.CommonName,
			"not_before": c.NotBefore.Format(time.RFC3339),
			"not_after":  c.NotAfter.Format(time.RFC3339),
			"sans":       sanSlice(c),
		}
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if m := titleRe.FindSubmatch(body); m != nil {
		res.Title = strings.TrimSpace(string(m[1]))
	}
	res.Header = resp.Header
	res.Body = body
	return res, nil
}

func sanSlice(c *x509.Certificate) []string {
	out := []string{}
	for _, dns := range c.DNSNames {
		out = append(out, dns)
	}
	return out
}
