# LOG_LEVEL 环境变量控制与 Docker 集成设计方案

- 日期：2026-08-03
- 状态：已评审通过
- 目标：支持从环境变量 `LOG_LEVEL` 读取配置动态设置终端日志输出级别，并在 `docker-compose.yml` 中集成 `LOG_LEVEL` 配置。

## 1. 详细设计

### 1.1 `cmd/atlas/main.go`
在 `main()` 启动初始化阶段，检查 `LOG_LEVEL` 环境变量：
```go
if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
    logger.SetLevel(lvl)
}
```

### 1.2 `docker-compose.yml`
在 `atlas` 和 `atlas2` 服务环境变量中配置 `LOG_LEVEL: "debug"`。

## 2. 影响文件

- `cmd/atlas/main.go`
- `docker-compose.yml`
