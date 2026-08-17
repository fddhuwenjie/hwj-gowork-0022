# 本质评测环境说明

## 项目

- 项目编号：`hwj-gowork-0022`
- 项目名称：古籍脱酸处理批次服务
- 项目说明：管理古籍酸度检测、脱酸处理批次、连续处理步骤、复测结论和待处理汇总。

## 固定环境

- Go toolchain：`go1.26.5`
- go.mod language version：`go 1.21`
- GOTOOLCHAIN：`local`
- 支持平台：`linux/amd64`、`linux/arm64`
- Docker 基础镜像：`golang:1.26.5-bookworm`
- Docker manifest：`golang@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd`

## 构建

```bash
./build_benzhi_docker.sh hwj-gowork-0022:benzhi-amd64 linux/amd64
./build_benzhi_docker.sh hwj-gowork-0022:benzhi-arm64 linux/arm64
```

## 运行

```bash
docker run --rm -it --network none hwj-gowork-0022:benzhi-amd64 bash
```

## 容器内验证

```bash
go version
go env GOTOOLCHAIN GOPROXY GOMODCACHE GOCACHE
go test ./...
go vet ./...
go build ./...
```

---

# 项目 README 同步内容

# 古籍脱酸处理批次服务

本服务用于管理馆藏古籍册的脱酸处理流程，包括酸度检测、按纸张材质组批、依次完成预检、脱酸、干燥和复测，并支持查询仍需复测或复测不合格的馆藏册。

## 功能特性

- 馆藏册登记与酸度检测记录
- 按纸张材质自动组批，同一馆藏册不能同时存在于两个未结束批次
- 批次状态严格按预检 → 脱酸 → 干燥 → 复测 → 关闭推进
- 复测读数必须晚于干燥完成时间，且 pH 达到 7.0 阈值方可关闭批次
- 关闭批次时全部复测合格才允许关闭，失败不改变任何状态
- 批次摘要按材质和复测结论（合格/不合格/待复测）汇总
- 查询所有仍需复测或复测不合格的馆藏册
- 标准库 HTTP JSON API，线程安全内存存储，无外部依赖

## 目录结构

```
.
├── cmd/server/main.go          # 服务入口
├── internal/domain             # 领域模型与常量
├── internal/store              # 线程安全内存存储
├── internal/service            # 业务逻辑与状态机
├── internal/httpapi            # HTTP JSON API
├── go.mod
└── README.md
```

## 运行

```bash
go run ./cmd/server
```

服务默认监听 `:8080`。

## API 示例

### 登记馆藏册

```bash
curl -X POST http://localhost:8080/api/items \
  -H "Content-Type: application/json" \
  -d '{"title":"古籍一","material":"宣纸"}'
```

### 记录酸度检测（可选）

```bash
curl -X POST http://localhost:8080/api/detections \
  -H "Content-Type: application/json" \
  -d '{"item_id":"item-1","ph":5.2,"detected_at":"2025-01-01T10:00:00Z"}'
```

### 按材质创建批次

```bash
curl -X POST http://localhost:8080/api/batches \
  -H "Content-Type: application/json" \
  -d '{"material":"宣纸"}'
```

### 完成当前步骤

```bash
curl -X POST http://localhost:8080/api/batches/{batchID}/steps/precheck/complete
curl -X POST http://localhost:8080/api/batches/{batchID}/steps/deacidify/complete
curl -X POST http://localhost:8080/api/batches/{batchID}/steps/drying/complete
```

注意：复测步骤不能通过 `complete` 完成，必须提交所有复测记录后关闭批次。

### 提交复测记录

```bash
curl -X POST http://localhost:8080/api/batches/{batchID}/retests \
  -H "Content-Type: application/json" \
  -d '{"item_id":"item-1","ph":7.5,"retested_at":"2025-01-02T10:00:00Z"}'
```

### 关闭批次

```bash
curl -X POST http://localhost:8080/api/batches/{batchID}/close
```

### 查询批次摘要

```bash
curl http://localhost:8080/api/batches/{batchID}/summary
```

### 查询需复测或复测不合格的馆藏册

```bash
curl http://localhost:8080/api/items/needing-retest
```

## 测试

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

## 容器构建

仓库中包含 `Dockerfile`、`benzhi.Dockerfile`、`.dockerignore` 以及可执行脚本 `build_docker.sh`、`build_benzhi_docker.sh`，可用于构建 linux/amd64 和 linux/arm64 镜像，并支持无网络运行验收。
