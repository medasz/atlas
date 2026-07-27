package scan

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsePortSpec 解析端口规格：支持 "80,443,8080-8090" 与 "1-1000"
func ParsePortSpec(spec string) ([]int, error) {
	if spec == "" {
		return nil, nil
	}
	seen := map[int]bool{}
	out := []int{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil || lo > hi {
				return nil, fmt.Errorf("invalid port range %q", part)
			}
			for p := lo; p <= hi; p++ {
				if !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
			}
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid port %q", part)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// commonHTTPPorts 视作 HTTP 服务的端口（探测时尝试 HTTP/HTTPS）
var commonHTTPPorts = map[int]bool{
	80:   true,
	443:  true,
	8080: true,
	8000: true,
	8443: true,
	8888: true,
	9000: true,
	7001: true,
	3128: true,
}
