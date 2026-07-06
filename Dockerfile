# ---- Build stage ----
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY shared/ shared/
COPY cmd/ cmd/

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/aiops-server ./cmd/server
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/aiops-agent  ./cmd/agent

# Cross-compile agents for distribution (one-click install)
RUN GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o /out/dist/aiops-agent.exe          ./cmd/agent && \
    GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o /out/dist/aiops-agent-darwin-arm64 ./cmd/agent && \
    cp /out/aiops-agent /out/dist/aiops-agent-linux-amd64

# ---- Server image ----
FROM alpine:3.20 AS server
RUN apk add --no-cache ca-certificates tzdata zip
WORKDIR /app

COPY --from=builder /out/aiops-server /app/aiops-server
COPY --from=builder /out/dist/ /app/dist/
COPY plugins/ /tmp/plugins/
RUN cd /tmp && zip -r /app/dist/plugins.zip plugins/ && rm -rf /tmp/plugins

EXPOSE 8080
VOLUME ["/app/data"]

ENTRYPOINT ["/app/aiops-server"]
CMD ["-addr", ":8080", "-config", "/app/data/server_config.json", "-dist", "/app/dist"]

# ---- Agent image ----
FROM alpine:3.20 AS agent
RUN apk add --no-cache ca-certificates tzdata python3 py3-pip
WORKDIR /app

COPY --from=builder /out/aiops-agent /app/aiops-agent
COPY plugins/ /app/plugins/

RUN pip3 install --no-cache-dir --break-system-packages psutil 2>/dev/null || true

ENTRYPOINT ["/app/aiops-agent"]
CMD ["--server", "http://aiops-server:8080"]
