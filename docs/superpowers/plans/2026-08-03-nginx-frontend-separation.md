# Nginx 独立前端托管与 Go 纯 API 引擎解耦 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 重构 Dockerfile 为多目标构建，将前端 Vue SPA 迁移至独立 Nginx 镜像托管，Go 后端移除 `-webdir` 专注于网络测绘 API 引擎。

## Global Constraints

- 不打破全量后端单元测试通过的逻辑。
- 保证 `docker-compose` 零依赖完美一键拉起。

---

### Task 1: 更新 `configs/nginx.conf`

**Files:**
- Modify: `configs/nginx.conf`

- [ ] **Step 1: 完善 configs/nginx.conf 静态托管与代理**

包含 `location /` 静态托管与 fallback 到 `/index.html`，以及 `location /api/` 反向代理。

---

### Task 2: 重构 `Dockerfile` 为多目标构建 (`backend` 和 `frontend`)

**Files:**
- Modify: `Dockerfile`

- [ ] **Step 1: 修改 Dockerfile 声明 AS backend 和 AS frontend 镜像**

---

### Task 3: 更新 `docker-compose.yml` 节点映射

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: 为 atlas 配置 target: backend，为 web 配置 target: frontend**

---

### Task 4: 全量测试验证

- [ ] **Step 1: 运行 `go test ./...` 验证代码无破坏**
