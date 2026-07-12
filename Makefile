# ============================================================
# 本地安全门 —— 每次 push 前 `make audit` 跑一遍，等价 CI。
# 单人开发的自查闸门：把问题挡在提交前，而非等 CI 变红。
# 首次使用先 `make tools` 安装工具（Windows 可在 git-bash 里跑）。
# ============================================================
GO ?= go

.PHONY: audit vet test vuln sec staticcheck sbom tools build fmt help

help: ## 显示可用目标
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n",$$1,$$2}'

audit: vet test vuln sec ## 跑全套安全门（vet+race测试+漏洞+SAST）

build: ## 编译 server 与 agent
	$(GO) build ./cmd/server ./cmd/agent

vet: ## go vet 静态检查
	$(GO) vet ./...

test: ## 单测（可移植，无需 cgo）
	$(GO) test ./...

race: ## race 竞态检测（需 CGO_ENABLED=1 + gcc/clang）
	CGO_ENABLED=1 $(GO) test -race ./...

vuln: ## 依赖 + stdlib 已知漏洞扫描（govulncheck）
	govulncheck ./...

sec: ## gosec 安全静态分析（中危及以上）
	gosec -exclude-dir=vendor -severity medium -confidence medium ./...

staticcheck: ## staticcheck 深度静态分析
	staticcheck ./...

sbom: ## 生成 CycloneDX SBOM（供企业交付/审计）
	cyclonedx-gomod mod -json -output sbom.cyclonedx.json

tools: ## 安装本地安全工具链
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	$(GO) install github.com/securego/gosec/v2/cmd/gosec@latest
	$(GO) install honnef.co/go/tools/cmd/staticcheck@latest
	$(GO) install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest
