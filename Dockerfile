# ---- Go 构建 ----
# 注意：go.mod 要求 go >= 1.25，需使用不低于该版本的镜像，并固定 GOTOOLCHAIN=local
# 以避免构建时联网下载工具链（受限网络下会失败）。
FROM golang:1.25 AS gobuild
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY
ENV GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/atlas ./cmd/atlas

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
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
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
