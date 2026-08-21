# 多阶段构建：构建层与运行层分离，减小镜像体积、减少攻击面（大厂镜像规范）。
FROM golang:1.22-alpine AS builder
WORKDIR /src
# 国内构建走 goproxy.cn，避免 proxy.golang.org 超时。
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod ./
RUN go mod download
COPY . .
# 静态编译，CGO_DISABLE 便于在 alpine/scratch 运行；注入构建版本与 buggy 标志。
ARG VERSION=sre-demo
ARG BUGGY=false
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.buildVersion=${VERSION} -X github.com/sre-demo/ordersvc/internal/handler.buggy=${BUGGY}" \
    -o /out/ordersvc ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /out/ordersvc /ordersvc
# 非 root 运行，符合容器安全基线。
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/ordersvc"]
