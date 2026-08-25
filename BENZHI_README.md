# BENZHI_README

基于 Go 实现的bioacoustic-release-hub HTTP API 项目，一款后端服务，已完整实现野生动物声学样本从登记、信号质检、标注争议裁决、整改复验、候选冻结到不可变研究放行的 JSON HTTP 服务，并提供 SQLite WAL 持久化、幂等写入、审计时间线和真实回环自检。

## 项目说明
- 项目：benzhi-project-bd7fd281-74ab-4bc8-a153-3249cea7c433
- 项目用途：已完整实现野生动物声学样本从登记、信号质检、标注争议裁决、整改复验、候选冻结到不可变研究放行的 JSON HTTP 服务，并提供 SQLite WAL 持久化、幂等写入、审计时间线和真实回环自检。
- Go 工具链：`golang:1.23.0`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-bd7fd281-74ab-4bc8-a153-3249cea7c433-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-bd7fd281-74ab-4bc8-a153-3249cea7c433-arm64 linux/arm64
docker run -it benzhi-project-bd7fd281-74ab-4bc8-a153-3249cea7c433-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck`
