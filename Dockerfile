# ---- Go 后端构建 ----
# 注意：go.mod 要求 go >= 1.25，需使用不低于该版本的镜像，并固定 GOTOOLCHAIN=local
FROM golang:1.25 AS gobuild
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY
ENV GOTOOLCHAIN=local
WORKDIR /src
RUN apt-get update && apt-get install -y libpcap-dev && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -tags raw_capture -o /out/atlas ./cmd/atlas

# ---- Vue 前端构建 ----
FROM node:20 AS webbuild
ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV npm_config_registry=$NPM_REGISTRY
WORKDIR /web
COPY web/package.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- 运行镜像 1: 纯 Go 后端测绘引擎 (backend) ----
FROM debian:bookworm-slim AS backend
ENV TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y ca-certificates libpcap0.8 tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=gobuild /out/atlas /app/atlas
COPY configs /app/configs
COPY migrations /app/migrations
EXPOSE 8080
ENTRYPOINT ["/app/atlas", \
            "-migrations", "/app/migrations", \
            "-rules", "/app/configs/fingerprint-rules.yaml"]

# ---- 运行镜像 2: 独立 Nginx 前端 Web 托管 (frontend) ----
FROM nginx:alpine AS frontend
ENV TZ=Asia/Shanghai
COPY --from=webbuild /web/dist /usr/share/nginx/html
COPY configs/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
