# 多阶段构建：构建层与运行层分离，减小镜像体积、减少攻击面（与 ordersvc Dockerfile 规范一致）。
FROM golang:1.22-alpine AS builder
WORKDIR /src
# 国内构建走 goproxy.cn，避免 proxy.golang.org 超时。
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=slo-operator-v1
# 静态编译，CGO_DISABLED 便于在 alpine/scratch 运行；注入构建版本。
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/manager ./cmd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /out/manager /manager
# 非 root 运行。
USER 65532:65532
ENTRYPOINT ["/manager"]
