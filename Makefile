.PHONY: help dev-backend dev-frontend dev install-frontend build-frontend build-backend build clean docker docker-run tidy

BIN := bin/gogitit
GO_FILES := $(shell find . -name '*.go' -not -path './web/*')

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

install-frontend: ## Install frontend dependencies
	cd web && npm install

tidy: ## go mod tidy
	go mod tidy

dev-backend: ## Run Go backend in dev mode (auth disabled)
	GOGITIT_AUTH_ENABLED=false go run ./cmd/gogitit

dev-frontend: ## Run Vite dev server (proxies /api to :8080)
	cd web && npm run dev

dev: ## Print the two dev commands to run in separate terminals
	@echo "Run in two terminals:"
	@echo "  make dev-backend"
	@echo "  make dev-frontend"

build-frontend: ## Build Svelte SPA into web/dist
	cd web && npm install && npm run build

build-backend: ## Build Go binary
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN) ./cmd/gogitit

build: build-frontend build-backend ## Full release build (frontend + backend)

clean: ## Clean build artifacts
	rm -rf bin web/dist data

docker: ## Build Docker image
	docker build -t gogitit:latest .

docker-run: ## Run image locally (auth disabled, port 8080)
	docker run --rm -p 8080:8080 \
	  -e GOGITIT_AUTH_ENABLED=false \
	  -v $(PWD)/data:/data \
	  -e GOGITIT_STORAGE_REPOS_DIR=/data/repos \
	  -e GOGITIT_STORAGE_STATE_DIR=/data/state \
	  gogitit:latest
