# BENZHI_README

这是一个基于 Go 实现的后端服务，用于承载 go-label-yanji-bridge-commissioning-g11-v1 的业务处理、数据管理与稳定运行。

## 项目说明

- 项目：VanceMichael/go-label-yanji-bridge-commissioning-g11-v1
- 项目用途：BridgeWatch coordinates the closeout, load testing, handover acceptance and opening-readiness work for a major bridge project. It is a Go HTTP service backed by SQLite and designed around auditable transactions, optimistic concurrency and recoverable background jobs.
- Go 工具链：`golang:1.25.0`
- 前端工具链：无

## 标准构建、运行和测试命令

进入容器后执行：

```bash
# 编译
cd '/app' && GOTOOLCHAIN=local go build ./...

# 启动
cd '/app' && GOTOOLCHAIN=local go run ./cmd/server

# 测试
cd '/app' && GOTOOLCHAIN=local go test ./...
```

## Docker 构建和进入容器

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-task-387-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-task-387-arm64 linux/arm64
docker run -it benzhi-task-387-amd64:latest
docker run -it --platform linux/arm64 benzhi-task-387-arm64:latest
```

## 题目验证命令

1. 预期退出码 0：`go test ./internal/httpapi -run '^TestCreateProjectFailureLeavesNoDurableState$' -count=1`
2. 预期退出码 0：`go test ./...`
3. 预期退出码 0：`GOTOOLCHAIN=local go build -buildvcs=false ./... && GOTOOLCHAIN=local go vet ./...`
