# ---- Go 构建 ----
# 注意：go.mod 要求 go >= 1.25，需使用不低于该版本的镜像，并固定 GOTOOLCHAIN=local
# 以避免构建时联网下载工具链（受限网络下会失败）。
FROM golang:1.25 AS gobuild
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY
ENV GOTOOLCHAIN=local
WORKDIR /src
# raw 抓包（SYN/ACK/FIN/Null/Xmas）依赖 gopacket/pcap（CGO），构建期需 libpcap 头文件。
RUN apt-get update && apt-get install -y libpcap-dev && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 启用 raw_capture 构建标签并打开 CGO；默认构建（无该 tag）下 raw 模式会优雅降级为 connect。
RUN CGO_ENABLED=1 go build -tags raw_capture -o /out/atlas ./cmd/atlas

# ---- 前端构建 ----
FROM node:20 AS webbuild
ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV npm_config_registry=$NPM_REGISTRY
WORKDIR /web
COPY web/package.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- 运行镜像 ----
FROM debian:bookworm-slim
# libpcap0.8 为 raw 抓包二进制的运行时动态链接依赖（缺它容器无法启动）。
RUN apt-get update && apt-get install -y ca-certificates libpcap0.8 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=gobuild /out/atlas /app/atlas
COPY --from=webbuild /web/dist /app/web/dist
COPY configs /app/configs
COPY migrations /app/migrations
EXPOSE 8080
ENTRYPOINT ["/app/atlas", "-config", "/app/configs/atlas.yaml", \
            "-migrations", "/app/migrations", \
            "-rules", "/app/configs/fingerprint-rules.yaml", \
            "-webdir", "/app/web/dist"]
