.PHONY: all build build-platform build-agent clean test proto web

# 默认目标
all: proto build

# 构建
build: build-platform build-agent-linux

build-platform:
	cd cmd/platform && go build -o ../../bin/platform .

build-agent:
	CGO_ENABLED=0 go build -o ../../bin/agent ./cmd/agent/

# 交叉编译 Agent（静态链接，兼容旧 glibc）
build-agent-linux: build-agent-linux-amd64 build-agent-linux-arm64

build-agent-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../bin/agent-linux-amd64 ./cmd/agent/

build-agent-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../../bin/agent-linux-arm64 ./cmd/agent/

# 生成 protobuf 代码
proto:
	protoc --go_out=. --go_opt=module=github.com/cylism/cylism-manager \
	       --go-grpc_out=. --go-grpc_opt=module=github.com/cylism/cylism-manager \
	       -I. api/proto/agent.proto

# 构建前端
web:
	cd web && npm run build

# 运行测试
test:
	go test ./... -v -count=1

# 清理
clean:
	rm -rf bin/
	rm -f data/cylism.db

# 开发运行
run-platform:
	go run ./cmd/platform/

run-agent:
	go run ./cmd/agent/

# 格式化
fmt:
	go fmt ./...
