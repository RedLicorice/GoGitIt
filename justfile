# GoGitIt — task runner (mirrors Makefile)

BIN := "bin/gogitit"

# Show available recipes
default:
    @just --list

# Install frontend dependencies
install-frontend:
    cd web && npm install

# go mod tidy
tidy:
    go mod tidy

# Run Go backend in dev mode (auth disabled, :8080)
dev-backend:
    GOGITIT_AUTH_ENABLED=false go run ./cmd/gogitit

# Run Vite dev server (proxies /api to :8080, :5173)
dev-frontend:
    cd web && npm run dev

# Run backend + frontend in one terminal (multitail merges both streams;
# backend is the noisy one, vite's few lines slot in). Quit with 'q'.
# ponytail: multitail SIGTERMs the shells on quit; a stray `go run` child can
# outlive it — `pkill -f 'go run ./cmd/gogitit'` if a port stays held.
dev:
    multitail -cT ANSI -l "GOGITIT_AUTH_ENABLED=false go run ./cmd/gogitit" \
              -cT ANSI -L "cd web && npm run dev"

# Build Svelte SPA into web/dist
build-frontend:
    cd web && npm install && npm run build

# Build Go binary
build-backend:
    mkdir -p bin
    CGO_ENABLED=0 go build -ldflags="-s -w" -o {{BIN}} ./cmd/gogitit

# Full release build (frontend + backend)
build: build-frontend build-backend

# Clean build artifacts
clean:
    rm -rf bin web/dist data

# Build Docker image
docker:
    docker build -t gogitit:latest .

# Run image locally (auth disabled, port 8080)
docker-run:
    docker run --rm -p 8080:8080 \
      -e GOGITIT_AUTH_ENABLED=false \
      -v $(pwd)/data:/data \
      -e GOGITIT_STORAGE_REPOS_DIR=/data/repos \
      -e GOGITIT_STORAGE_STATE_DIR=/data/state \
      gogitit:latest
