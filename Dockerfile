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
# ---- Build stage ----
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY shared/ shared/
COPY cmd/ cmd/

# Build server
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/aiops-server ./cmd/server

# Build agent (Linux amd64 by default)
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/aiops-agent ./cmd/agent

# ---- Server image ----
FROM alpine:3.20 AS server
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /out/aiops-server /app/aiops-server
COPY --from=builder /out/aiops-agent /app/dist/aiops-agent-linux-amd64
COPY plugins/ /app/dist/plugins/

# Create dist/plugins.zip for one-click install
RUN apk add --no-cache zip && \
    cd /app/dist && zip -r plugins.zip plugins/ && \
    rm -rf plugins/ && \
    apk del zip

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
# syntax=docker/dockerfile:1
# ---------- build stage ----------
FROM golang:1.22-alpine AS build
RUN apk add --no-cache zip
WORKDIR /src
COPY . .
ENV CGO_ENABLED=0
# Build the server and the downloadable agent binaries for every platform, and
# bundle the plugins — so the one-line install (/dl) works out of the box.
RUN mkdir -p /out/dist \
 && go build -trimpath -ldflags "-s -w" -o /out/aiops-server ./cmd/server \
 && GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/dist/aiops-agent-linux-amd64  ./cmd/agent \
 && GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/dist/aiops-agent.exe          ./cmd/agent \
 && GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o /out/dist/aiops-agent-darwin-arm64 ./cmd/agent \
 && (cd /src && zip -rq /out/dist/plugins.zip plugins -x 'plugins/.*')

# ---------- runtime stage ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 aiops
WORKDIR /app
COPY --from=build /out/aiops-server /app/aiops-server
COPY --from=build /out/dist /app/dist
RUN mkdir -p /app/data && chown -R aiops /app/data
ENV LANG=C.UTF-8 TZ=Asia/Shanghai
USER aiops
EXPOSE 8080
# server_config.json (account / alerts / checks) persists in this volume
VOLUME ["/app/data"]
ENTRYPOINT ["/app/aiops-server", "-addr", ":8080", "-config", "/app/data/server_config.json", "-dist", "/app/dist"]
