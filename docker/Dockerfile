# syntax=docker/dockerfile:1
# ============================================================================
# Multi-arch (linux/amd64 + linux/arm64) build with version injection.
# - BASE_REGISTRY defaults to the Huawei SWR mirror (fast for local/China builds);
#   CI passes BASE_REGISTRY=docker.io to use the official multi-arch base images.
# - VERSION is injected into main.appVersion (set to the git tag by the release CI).
# - The Go builds are cross-compiled on the native build host (CGO off), so an
#   arm64 image does NOT pay for QEMU emulation of the (slow) Go compile.
# ============================================================================
ARG BASE_REGISTRY=swr.cn-east-3.myhuaweicloud.com/sreyun/docker.io
# GO_VERSION 默认 1.22（SWR 镜像仓现只有此版本，保证本地构建可用）。
# ⚠️ 安全：Go 1.22 已 EOL，含 crypto/tls CVE(GO-2026-5856)。发布版应升级：
#   先把 golang:1.26-alpine 同步到 SWR，再改此默认；或用官方源构建：
#   docker build --build-arg BASE_REGISTRY=docker.io --build-arg GO_VERSION=1.26 ...
ARG GO_VERSION=1.22

# ---- Build stage (always runs on the native BUILDPLATFORM) ----
FROM --platform=$BUILDPLATFORM ${BASE_REGISTRY}/golang:${GO_VERSION}-alpine AS builder
WORKDIR /src
# TARGETOS/TARGETARCH are provided automatically by buildx per target platform.
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY shared/ shared/
COPY cmd/ cmd/
# 将配置示例复制到 cmd/agent/ 以供 go:embed 内嵌（.gitignore 排除，构建时复制）
COPY config.example.yaml cmd/agent/config_example.yaml

# Server + agent binaries for THIS image's target arch (native cross-compile).
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOFLAGS=-mod=vendor \
    go build -ldflags="-s -w -X main.appVersion=${VERSION}" -o /out/aiops-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOFLAGS=-mod=vendor \
    go build -ldflags="-s -w -X main.appVersion=${VERSION}" -o /out/aiops-agent  ./cmd/agent

# All-platform agent binaries for the one-click install distribution (served by the
# server via /dl/, so they must cover every platform regardless of the image arch).
RUN set -e; LD="-s -w -X main.appVersion=${VERSION}"; \
    GOOS=linux   GOARCH=amd64 GOFLAGS=-mod=vendor go build -ldflags="$LD" -o /out/dist/aiops-agent-linux-amd64  ./cmd/agent; \
    GOOS=linux   GOARCH=arm64 GOFLAGS=-mod=vendor go build -ldflags="$LD" -o /out/dist/aiops-agent-linux-arm64  ./cmd/agent; \
    GOOS=darwin  GOARCH=amd64 GOFLAGS=-mod=vendor go build -ldflags="$LD" -o /out/dist/aiops-agent-darwin-amd64 ./cmd/agent; \
    GOOS=darwin  GOARCH=arm64 GOFLAGS=-mod=vendor go build -ldflags="$LD" -o /out/dist/aiops-agent-darwin-arm64 ./cmd/agent; \
    GOOS=windows GOARCH=amd64 GOFLAGS=-mod=vendor go build -ldflags="$LD" -o /out/dist/aiops-agent.exe          ./cmd/agent

# Optional agent-binary compression. UPX roughly halves the /dl download (7.5MB -> ~3MB).
# DEFAULT OFF: a packed binary can trip AV/EDR heuristics, and for a security-monitoring
# agent a false-positive quarantine (agent deleted, monitoring dies) is worse than 4MB.
# Enable with:  docker build --build-arg UPX=1 ...
# Only linux-amd64 + windows-amd64 are packed — UPX Mach-O(darwin) support is unreliable,
# and arm64 packing is best avoided; those stay uncompressed.
ARG UPX=0
RUN if [ "$UPX" = "1" ]; then \
      apk add --no-cache upx && \
      upx --best --lzma /out/dist/aiops-agent-linux-amd64 /out/dist/aiops-agent.exe && \
      echo "[build] UPX applied to linux-amd64 + windows-amd64"; \
    else echo "[build] UPX skipped (opt-in via --build-arg UPX=1)"; fi

# ---- Server image ----
FROM ${BASE_REGISTRY}/alpine:3.20 AS server
RUN apk add --no-cache ca-certificates tzdata zip
WORKDIR /app

COPY --from=builder /out/aiops-server /app/aiops-server
COPY --from=builder /out/dist/ /app/dist/
COPY config.example.yaml /app/dist/config.example.yaml
COPY plugins/ /tmp/plugins/
RUN cd /tmp && zip -r /app/dist/plugins.zip plugins/ && rm -rf /tmp/plugins

EXPOSE 8529
HEALTHCHECK --interval=15s --timeout=5s --retries=3 CMD wget -qO- http://127.0.0.1:8529/healthz || exit 1
VOLUME ["/app/data"]

ENTRYPOINT ["/app/aiops-server"]
CMD ["-addr", ":8529", "-config", "/app/data/server_config.json", "-dist", "/app/dist"]

# ---- Agent image ----
FROM ${BASE_REGISTRY}/alpine:3.20 AS agent
RUN apk add --no-cache ca-certificates tzdata python3 py3-pip
WORKDIR /app

COPY --from=builder /out/aiops-agent /app/aiops-agent
COPY plugins/ /app/plugins/

RUN pip3 install --no-cache-dir --break-system-packages psutil 2>/dev/null || true

ENTRYPOINT ["/app/aiops-agent"]
CMD ["--server", "http://aiops-server:8529"]
