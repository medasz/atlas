# 端口多模式探测 + HTTP 探测

## Description
实现端口探测（多模式可配置）与 HTTP 探测（仅对识别为 HTTP 的服务）。统一接口 `Probe(ip, ports, mode, opts)`，支持 connect/syn/fin/null/xmas/udp/ack；syn/udp 等需 raw socket 的模式基于 gopacket+libpcap（Linux），不支持时回退 connect。HTTP 探测以指纹 service 字段为门控。

## Acceptance Criteria
- [ ] 实现 `Probe(ip, ports, mode, opts)`，支持 connect / syn / fin / null / xmas / udp / ack；默认 connect
- [ ] syn / udp / 隐蔽等需 raw socket 模式在 Linux+libpcap 下可用，不支持环境回退 connect 并告警
- [ ] 端口范围可配置：top1000 / 指定列表 / 指定区间 / 全端口 1..65535，默认 top1000
- [ ] 开放端口记录 port + proto + banner
- [ ] HTTP 探测仅对 service=http/https 端口执行；常见 Web 端口（80/443/8080/8000/8443 等）未识别服务时补充策略可关
- [ ] 抓取 title / 响应头 / 状态码 / body（截断 ≤256KB），支持 gzip 解压与 charset 识别，写 webinfo
- [ ] 单 IP 探测在并发上限内完成，不突破全局限速
- [ ] 类型检查与 lint 通过

## Dependencies
Issue #3

## Type
backend

## Priority
high

## SPEC Reference
2.2, 5.1
