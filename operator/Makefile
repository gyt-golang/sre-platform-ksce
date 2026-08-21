# SLO Operator Makefile —— 本地编译验证与构建辅助。
# 远程构建由 bootstrap-ksce.sh 调用 docker build（Dockerfile 多阶段），不依赖此 Makefile 的 docker 目标。

.PHONY: build test vet manifests

# 本地编译验证（开发期用）。
build:
	go build ./...

# 静态检查。
vet:
	go vet ./...

# 单测（如有）。
test:
	go test ./... -cover

# CRD/深拷贝代码生成提示（本项目手写等价文件，无需 controller-gen）。
# 如需重新生成：安装 controller-gen 后跑 controller-gen crd:crdVersions=v1 paths=./api/...
manifests:
	@echo "CRD/类型已手写等价实现（api/v1/zz_generated.deepcopy.go + deploy/manifests/slo-operator-crd.yaml）"
	@echo "如需 controller-gen 重新生成：controller-gen object:headerFile=hack/boilerplate.go.txt paths=./api/v1/..."
