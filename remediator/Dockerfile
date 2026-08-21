# 多阶段构建：remediator 服务 + kubectl（rollout undo 动作需要）。
FROM golang:1.22-alpine AS builder
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=remediator-v1
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.buildVersion=${VERSION}" \
    -o /out/remediator ./cmd/server

FROM alpine:3.20
# kubectl 用于 rollout undo 动作（argoproj rollouts abort 走 kubectl patch）。
RUN apk add --no-cache ca-certificates tzdata curl && \
    curl -fsSL -o /usr/local/bin/kubectl \
    "https://dl.k8s.io/release/v1.31.0/bin/linux/amd64/kubectl" && \
    chmod +x /usr/local/bin/kubectl
COPY --from=builder /out/remediator /remediator
USER 65532:65532
ENTRYPOINT ["/remediator"]
