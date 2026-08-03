# Nginx 独立前端托管与 Go 纯 API 引擎解耦设计方案

- 日期：2026-08-03
- 状态：已评审通过
- 目标：将前端 Vue SPA 完全迁移至专用 Nginx 镜像托管，Go 后端移除 `-webdir` 专注于 REST API 与高并发发包，实现真正的前后端物理解耦。

## 1. 详细设计

### 1.1 Dockerfile 多目标构建
将 Dockerfile 拆分为多目标构建阶段：

- **`backend` 目标镜像**：
  - 基于 `debian:bookworm-slim` + `tzdata` + `libpcap0.8`。
  - 拷贝二进制 `/out/atlas`、`configs`、`migrations`。
  - ENTRYPOINT 移除了 `-webdir` 参数。

- **`frontend` 目标镜像**：
  - 基于 `node:20` 构建 Vue 静态资源到 `/web/dist`。
  - 基于 `nginx:alpine` 拷贝产物至 `/usr/share/nginx/html`，并写入 `configs/nginx.conf`。

### 1.2 `configs/nginx.conf`
Nginx 配置文件配置：
- `location /`: 托管前端 Vue SPA 页面，带有 `try_files $uri $uri/ /index.html;`。
- `location /api/`: 反向代理 `http://host.docker.internal:8080`，并开启 WebSocket 支持。

### 1.3 `docker-compose.yml` 调整
- `atlas` 引擎：配置 `build.target: backend` 和 `network_mode: "host"`。
- `web`: 配置 `build.target: frontend` 和 `ports: - "8080:80"`。

## 2. 影响文件

- `Dockerfile`
- `configs/nginx.conf`
- `docker-compose.yml`
